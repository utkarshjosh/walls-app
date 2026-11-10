//! Is this file a live scene the app may keep? Pure.
//!
//! A scene is running code, so it is contained in layers (see
//! `docs/16-live-scene-standard.md`, "How the app contains a scene"). This is
//! layer 3: when a scene file arrives, it is kept only if it carries the
//! standard's content policy in a place where the web engine is certain to
//! apply it before any of the scene's code runs. That policy is what stops a
//! scene from loading or sending anything.
//!
//! "Carries the policy" is deliberately stricter than "the line appears in
//! the text". A policy line inside a comment, a `<template>` or the body, or
//! behind a second `content` attribute, would be in the file and ignored by
//! the engine. So nothing may come before the policy except what the standard
//! allows there: the doctype, the `<html>` and `<head>` tags and
//! `<meta charset="utf-8">`. With only those in front of it, every HTML
//! parser reaches the policy in the head with no script run yet.
//!
//! The check runs on arrival only. Scenes already in the library were saved
//! under an earlier policy text and keep playing.

use std::path::Path;

/// Rule 1 of the standard: the exact value of the policy's `content`.
pub const SCENE_POLICY: &str = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; base-uri 'none'; form-action 'none'";

/// What the person is told. One sentence, whatever the detail.
pub const REFUSAL: &str = "That live wallpaper is not in a form Still can run safely.";

/// Far above the standard's 512 KB for a scene. It only keeps a mislabelled
/// huge download from being read into memory to be checked.
pub const MAX_SCENE_BYTES: u64 = 4 * 1024 * 1024;

const POLICY_HEADER: &str = "content-security-policy";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SceneProblem {
    TooLarge,
    /// No `<meta http-equiv="Content-Security-Policy">` anywhere.
    NoPolicy,
    /// More than one; the standard allows exactly one.
    SeveralPolicies,
    /// The policy tag is there but its content is not the standard's, or the
    /// tag has repeated or extra attributes (an engine reads the first of a
    /// repeated attribute, which need not be the one that looks right).
    WrongPolicy,
    /// A `<script` comes before the policy.
    PolicyAfterScript,
    /// Something other than the doctype, `<html>`, `<head>` and the charset
    /// comes before the policy, so the engine might never apply it.
    PolicyNotFirst,
}

/// Files stored under these names are scenes: they are checked on arrival,
/// and only they may be played as a scene (which loads the file as a web page).
pub fn is_scene_name(name_or_path: &str) -> bool {
    Path::new(name_or_path)
        .extension()
        .and_then(|extension| extension.to_str())
        .is_some_and(|extension| extension.eq_ignore_ascii_case("html") || extension.eq_ignore_ascii_case("htm"))
}

/// Decide whether `html` is an acceptable scene; see the module comment.
pub fn check_scene(html: &[u8]) -> Result<(), SceneProblem> {
    if html.len() as u64 > MAX_SCENE_BYTES {
        return Err(SceneProblem::TooLarge);
    }
    // A UTF-8 byte-order mark may open the file; anything else there is content.
    let start = if html.starts_with(&[0xEF, 0xBB, 0xBF]) { 3 } else { 0 };

    let mut policies = meta_tags(html).filter(|(_, tag)| is_policy(tag));
    let Some((at, policy)) = policies.next() else {
        return Err(SceneProblem::NoPolicy);
    };
    if policies.next().is_some() {
        return Err(SceneProblem::SeveralPolicies);
    }

    // Exactly `http-equiv` and `content`, once each, in either order.
    let content = policy.attribute("content");
    if policy.repeated || policy.attributes.len() != 2 || content != Some(SCENE_POLICY.as_bytes()) {
        return Err(SceneProblem::WrongPolicy);
    }
    if find_tag(html, b"script", start).is_some_and(|script| script < at) {
        return Err(SceneProblem::PolicyAfterScript);
    }
    if !only_preamble(html, start, at) || !html[start..policy.end].iter().all(|byte| is_plain(*byte)) {
        return Err(SceneProblem::PolicyNotFirst);
    }
    Ok(())
}

/// True when `html[from..to]` holds nothing but white space, a doctype, the
/// `<html>` and `<head>` start tags and `<meta charset="utf-8">`.
fn only_preamble(html: &[u8], from: usize, to: usize) -> bool {
    let mut at = from;
    loop {
        while at < to && html[at].is_ascii_whitespace() {
            at += 1;
        }
        if at >= to {
            return true;
        }
        if starts_with_ignore_case(&html[at..to], b"<!doctype") {
            // A doctype runs to the next ">" and holds no markup of its own.
            let Some(length) = html[at..to].iter().position(|byte| *byte == b'>') else {
                return false;
            };
            if html[at + 1..at + length].contains(&b'<') {
                return false;
            }
            at += length + 1;
            continue;
        }
        let Some(tag) = parse_start_tag(html, at) else {
            return false;
        };
        let allowed = !tag.repeated
            && match tag.name.as_str() {
                "html" | "head" => true,
                "meta" => {
                    let charset = tag.attribute("charset");
                    tag.attributes.len() == 1
                        && charset.is_some_and(|value| {
                            value.eq_ignore_ascii_case(b"utf-8") || value.eq_ignore_ascii_case(b"utf8")
                        })
                }
                _ => false,
            };
        if !allowed || tag.end > to {
            return false;
        }
        at = tag.end;
    }
}

/// Printable ASCII and ordinary white space. Everything up to the end of the
/// policy tag must be this plain, so no character encoding an engine might
/// choose can read those bytes as anything else.
fn is_plain(byte: u8) -> bool {
    matches!(byte, 0x20..=0x7E | b'\t' | b'\n' | b'\r' | 0x0C)
}

fn is_policy(tag: &Tag) -> bool {
    tag.attribute("http-equiv").is_some_and(|value| value.eq_ignore_ascii_case(POLICY_HEADER.as_bytes()))
}

// ---------------------------------------------------------------------------
// A small reader for HTML start tags, following the HTML tokenizer's rules
// for where names and values begin and end.
// ---------------------------------------------------------------------------

struct Tag<'a> {
    /// Lower-case tag name.
    name: String,
    /// Lower-case attribute names with their values exactly as written.
    attributes: Vec<(String, &'a [u8])>,
    /// An attribute name appeared more than once.
    repeated: bool,
    /// Index just past the closing ">".
    end: usize,
}

impl<'a> Tag<'a> {
    /// The first attribute of that name, which is the one an engine uses.
    fn attribute(&self, name: &str) -> Option<&'a [u8]> {
        self.attributes.iter().find(|(known, _)| known == name).map(|(_, value)| *value)
    }
}

/// Every `<meta ...>` start tag anywhere in the text, with where it starts.
/// Deliberately blind to context: a policy tag inside a comment or a script
/// string still counts, which can only make the check stricter.
fn meta_tags<'a>(html: &'a [u8]) -> impl Iterator<Item = (usize, Tag<'a>)> + 'a {
    let mut from = 0;
    std::iter::from_fn(move || {
        while let Some(at) = find_tag(html, b"meta", from) {
            from = at + 1;
            if let Some(tag) = parse_start_tag(html, at) {
                return Some((at, tag));
            }
        }
        None
    })
}

/// The first `<name` at or after `from` that really is that tag (followed by
/// white space, "/" or ">"), ignoring case.
fn find_tag(html: &[u8], name: &[u8], from: usize) -> Option<usize> {
    let mut at = from;
    while at + 1 + name.len() < html.len() {
        if html[at] == b'<' && starts_with_ignore_case(&html[at + 1..], name) {
            let after = html[at + 1 + name.len()];
            if after.is_ascii_whitespace() || after == b'/' || after == b'>' {
                return Some(at);
            }
        }
        at += 1;
    }
    None
}

/// Read the start tag beginning at `html[at] == b'<'`. `None` if it is not a
/// start tag or the text ends before its ">".
fn parse_start_tag(html: &[u8], at: usize) -> Option<Tag<'_>> {
    let byte = |index: usize| html.get(index).copied();
    if byte(at) != Some(b'<') || !byte(at + 1)?.is_ascii_alphabetic() {
        return None;
    }
    let mut cursor = at + 1;
    let name_start = cursor;
    while !matches!(byte(cursor)?, b'/' | b'>') && !byte(cursor)?.is_ascii_whitespace() {
        cursor += 1;
    }
    let name = String::from_utf8_lossy(&html[name_start..cursor]).to_ascii_lowercase();
    let mut attributes: Vec<(String, &[u8])> = Vec::new();
    let mut repeated = false;

    loop {
        // Between attributes: white space and stray slashes are skipped.
        while byte(cursor)?.is_ascii_whitespace() || byte(cursor)? == b'/' {
            cursor += 1;
        }
        if byte(cursor)? == b'>' {
            return Some(Tag { name, attributes, repeated, end: cursor + 1 });
        }
        let attribute_start = cursor;
        cursor += 1; // A name may even begin with "=", as in the tokenizer.
        while !matches!(byte(cursor)?, b'/' | b'>' | b'=') && !byte(cursor)?.is_ascii_whitespace() {
            cursor += 1;
        }
        let attribute = String::from_utf8_lossy(&html[attribute_start..cursor]).to_ascii_lowercase();
        while byte(cursor)?.is_ascii_whitespace() {
            cursor += 1;
        }
        let mut value: &[u8] = &[];
        if byte(cursor)? == b'=' {
            cursor += 1;
            while byte(cursor)?.is_ascii_whitespace() {
                cursor += 1;
            }
            match byte(cursor)? {
                quote @ (b'"' | b'\'') => {
                    let value_start = cursor + 1;
                    cursor = value_start;
                    while byte(cursor)? != quote {
                        cursor += 1;
                    }
                    value = &html[value_start..cursor];
                    cursor += 1;
                }
                b'>' => {}
                _ => {
                    let value_start = cursor;
                    while byte(cursor)? != b'>' && !byte(cursor)?.is_ascii_whitespace() {
                        cursor += 1;
                    }
                    value = &html[value_start..cursor];
                }
            }
        }
        if attributes.iter().any(|(known, _)| *known == attribute) {
            repeated = true;
        } else {
            attributes.push((attribute, value));
        }
    }
}

fn starts_with_ignore_case(text: &[u8], prefix: &[u8]) -> bool {
    text.len() >= prefix.len() && text[..prefix.len()].eq_ignore_ascii_case(prefix)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Rule 1's line, character for character as the standard prints it.
    const POLICY_LINE: &str = r#"<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; base-uri 'none'; form-action 'none'">"#;

    /// A scene laid out the way the kit writes them, around `policy`.
    fn scene_with(policy: &str) -> String {
        format!(
            "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n{policy}\n\
             <meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n<title>Quiet Clock</title>\n\
             <!-- A live wallpaper scene. -->\n<style>html,body{{margin:0;background:#1c2230}}</style>\n</head>\n\
             <body>\n<canvas id=\"c\"></canvas>\n<script>\n(function () {{ var h = location.hash; }})();\n</script>\n</body>\n</html>\n"
        )
    }

    fn check(text: &str) -> Result<(), SceneProblem> {
        check_scene(text.as_bytes())
    }

    #[test]
    fn the_standards_line_is_the_constant() {
        assert!(POLICY_LINE.contains(&format!("content=\"{SCENE_POLICY}\"")));
    }

    #[test]
    fn the_exact_line_passes() {
        assert_eq!(check(&scene_with(POLICY_LINE)), Ok(()));
        // The barest acceptable file: the policy, then the scene.
        assert_eq!(check(&format!("{POLICY_LINE}<script>draw()</script>")), Ok(()));
        // A byte-order mark in front changes nothing.
        let mut with_mark = vec![0xEF, 0xBB, 0xBF];
        with_mark.extend_from_slice(scene_with(POLICY_LINE).as_bytes());
        assert_eq!(check_scene(&with_mark), Ok(()));
    }

    #[test]
    fn attribute_order_quotes_and_case_do_not_matter() {
        let swapped = format!(r#"<meta content="{SCENE_POLICY}" http-equiv="Content-Security-Policy">"#);
        assert_eq!(check(&scene_with(&swapped)), Ok(()));
        // The policy's own quotes are single, so only http-equiv can use them.
        let mixed_quotes = format!(r#"<meta http-equiv='Content-Security-Policy' content="{SCENE_POLICY}">"#);
        assert_eq!(check(&scene_with(&mixed_quotes)), Ok(()));
        let unquoted = format!(r#"<meta http-equiv=Content-Security-Policy content="{SCENE_POLICY}">"#);
        assert_eq!(check(&scene_with(&unquoted)), Ok(()));
        let shouting = format!(r#"<META HTTP-EQUIV="content-security-policy" CONTENT="{SCENE_POLICY}">"#);
        assert_eq!(check(&scene_with(&shouting)), Ok(()));
        let spaced = format!("<meta  http-equiv = \"Content-Security-Policy\"\n      content = \"{SCENE_POLICY}\" />");
        assert_eq!(check(&scene_with(&spaced)), Ok(()));
    }

    #[test]
    fn the_content_itself_must_match_exactly() {
        // The policy text scenes carried before the standard.
        let earlier = r#"<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'">"#;
        assert_eq!(check(&scene_with(earlier)), Err(SceneProblem::WrongPolicy));
        let variations = [
            SCENE_POLICY.replace("default-src 'none'", "default-src *"),
            SCENE_POLICY.replace("; form-action 'none'", ""),
            format!("{SCENE_POLICY};"),
            format!(" {SCENE_POLICY}"),
            SCENE_POLICY.to_uppercase(),
            format!("{SCENE_POLICY}; connect-src https:"),
            // An engine would decode this to the right text; the file must simply say it.
            SCENE_POLICY.replace('\'', "&#39;"),
            String::new(),
        ];
        for content in variations {
            let tag = format!(r#"<meta http-equiv="Content-Security-Policy" content="{content}">"#);
            assert_eq!(check(&scene_with(&tag)), Err(SceneProblem::WrongPolicy), "{content:?}");
        }
        let no_content = r#"<meta http-equiv="Content-Security-Policy">"#;
        assert_eq!(check(&scene_with(no_content)), Err(SceneProblem::WrongPolicy));
    }

    #[test]
    fn a_missing_policy_fails() {
        assert_eq!(check(&scene_with("")), Err(SceneProblem::NoPolicy));
        assert_eq!(check("<script>fetch('https://example.com')</script>"), Err(SceneProblem::NoPolicy));
        assert_eq!(check(""), Err(SceneProblem::NoPolicy));
        // The policy as text, not as a tag.
        assert_eq!(check(&format!("<p>{SCENE_POLICY}</p>")), Err(SceneProblem::NoPolicy));
        // A response header name in some other meta is not a policy.
        let other = scene_with(r#"<meta http-equiv="X-Content-Security-Policy" content="x">"#);
        assert_eq!(check(&other), Err(SceneProblem::NoPolicy));
    }

    #[test]
    fn two_policies_fail() {
        let twice = scene_with(&format!("{POLICY_LINE}\n{POLICY_LINE}"));
        assert_eq!(check(&twice), Err(SceneProblem::SeveralPolicies));
        let loose_second = format!(
            "{POLICY_LINE}\n<meta http-equiv=\"Content-Security-Policy\" content=\"default-src *\">"
        );
        assert_eq!(check(&scene_with(&loose_second)), Err(SceneProblem::SeveralPolicies));
        // Even one written into the body by hand later in the file.
        let later = scene_with(POLICY_LINE).replace("<canvas", &format!("{POLICY_LINE}<canvas"));
        assert_eq!(check(&later), Err(SceneProblem::SeveralPolicies));
    }

    #[test]
    fn a_policy_after_a_script_fails() {
        let script_first = format!("<!doctype html><html><head><script>early()</script>{POLICY_LINE}</head></html>");
        assert_eq!(check(&script_first), Err(SceneProblem::PolicyAfterScript));
        let upper = format!("<SCRIPT >early()</SCRIPT>{POLICY_LINE}");
        assert_eq!(check(&upper), Err(SceneProblem::PolicyAfterScript));
        // A word that merely starts with "script" is not a script tag...
        let harmless = format!("<scripture></scripture>{POLICY_LINE}");
        // ...but it is still something in front of the policy.
        assert_eq!(check(&harmless), Err(SceneProblem::PolicyNotFirst));
    }

    #[test]
    fn a_file_that_is_not_html_fails() {
        let jpeg = [0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, b'J', b'F', b'I', b'F', 0x00, 0x01];
        assert_eq!(check_scene(&jpeg), Err(SceneProblem::NoPolicy));
        assert_eq!(check("{\"title\": \"Quiet Clock\"}"), Err(SceneProblem::NoPolicy));
        // Binary junk in front of an otherwise perfect scene.
        let mut junk = vec![0x00, 0x1B, 0x24, 0x42];
        junk.extend_from_slice(scene_with(POLICY_LINE).as_bytes());
        assert_eq!(check_scene(&junk), Err(SceneProblem::PolicyNotFirst));
        let huge = vec![b' '; MAX_SCENE_BYTES as usize + 1];
        assert_eq!(check_scene(&huge), Err(SceneProblem::TooLarge));
    }

    /// The line is in the text each time, but the engine would not apply it
    /// (or not before something else ran), so the file does not carry it.
    #[test]
    fn a_policy_the_engine_would_ignore_fails() {
        let cases = [
            ("inside a comment", format!("<!-- {POLICY_LINE} --><script>run()</script>")),
            ("behind a comment that only looks open", format!("<!--><img src=x onerror=run()><!-- -->{POLICY_LINE}")),
            ("in the body", format!("<p>hello</p>{POLICY_LINE}<script>run()</script>")),
            ("inside a title", format!("<title>{POLICY_LINE}</title><script>run()</script>")),
            ("inside a template", format!("<template>{POLICY_LINE}</template><script>run()</script>")),
            ("inside a style", format!("<style>{POLICY_LINE}</style><script>run()</script>")),
            ("after an element with a handler", format!("<svg onload=run()></svg>{POLICY_LINE}")),
            ("after a redirect", format!("<meta http-equiv=\"refresh\" content=\"0;url=https://example.com\">{POLICY_LINE}")),
            ("after another charset", format!("<meta charset=\"iso-2022-jp\">{POLICY_LINE}")),
            ("after text", format!("hello {POLICY_LINE}")),
            ("after a viewport", format!("<meta name=\"viewport\" content=\"width=device-width\">{POLICY_LINE}")),
        ];
        for (what, text) in cases {
            assert_eq!(check(&text), Err(SceneProblem::PolicyNotFirst), "policy {what}");
        }
        // Text that is not plain ASCII before the policy, even inside an allowed tag.
        let odd = format!("<html lang=\"\u{3042}\">{POLICY_LINE}");
        assert_eq!(check(&odd), Err(SceneProblem::PolicyNotFirst));
    }

    #[test]
    fn a_repeated_or_extra_attribute_on_the_policy_fails() {
        // An engine uses the first `content`; this file hopes we read the last.
        let decoy = format!(r#"<meta http-equiv="Content-Security-Policy" content="default-src *" content="{SCENE_POLICY}">"#);
        assert_eq!(check(&scene_with(&decoy)), Err(SceneProblem::WrongPolicy));
        let reversed = format!(r#"<meta http-equiv="Content-Security-Policy" content="{SCENE_POLICY}" content="default-src *">"#);
        assert_eq!(check(&scene_with(&reversed)), Err(SceneProblem::WrongPolicy));
        let extra = format!(r#"<meta http-equiv="Content-Security-Policy" content="{SCENE_POLICY}" name="x">"#);
        assert_eq!(check(&scene_with(&extra)), Err(SceneProblem::WrongPolicy));
        // Two http-equiv values: the first one decides, and it is not a policy.
        let two_names = format!(r#"<meta http-equiv="refresh" http-equiv="Content-Security-Policy" content="{SCENE_POLICY}">"#);
        assert_eq!(check(&scene_with(&two_names)), Err(SceneProblem::NoPolicy));
    }

    #[test]
    fn the_usual_openings_are_accepted() {
        let openings = [
            "<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"UTF-8\">\n",
            "<!doctype html><meta charset=utf-8>",
            "<html lang='en' class=\"scene\"><head>",
            "\n\n  <head>\n\t<meta charset='utf8' />\n",
            "",
        ];
        for opening in openings {
            let text = format!("{opening}{POLICY_LINE}\n<title>t</title><script>draw()</script>");
            assert_eq!(check(&text), Ok(()), "{opening:?}");
        }
    }

    #[test]
    fn a_tag_cut_off_by_the_end_of_the_file_is_not_a_policy() {
        let cut = &POLICY_LINE[..POLICY_LINE.len() - 1];
        assert_eq!(check(cut), Err(SceneProblem::NoPolicy));
        assert_eq!(check("<meta http-equiv=\"Content-Security-Policy\" content=\""), Err(SceneProblem::NoPolicy));
        assert_eq!(check("<meta"), Err(SceneProblem::NoPolicy));
        assert_eq!(check("<"), Err(SceneProblem::NoPolicy));
    }

    #[test]
    fn scene_names_end_in_html() {
        for name in ["d11e93861ab6a.html", "Scene.HTML", "old.htm", "/lib/quiet-clock.html"] {
            assert!(is_scene_name(name), "{name}");
        }
        for name in ["a.jpg", "clip.mp4", "html", "scene.html.jpg", "scene.xhtml", "noextension", ""] {
            assert!(!is_scene_name(name), "{name}");
        }
    }
}
