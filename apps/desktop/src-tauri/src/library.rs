//! The library: every wallpaper file the app keeps, flat in one folder.
//!
//! Names are the only thing the UI can choose, so they are validated strictly
//! before they ever become part of a path. Files appear atomically (written to
//! a hidden temporary file, then renamed) so a crash or a failed download can
//! never leave a half-written wallpaper under a real name.
//!
//! A scene (an `.html` file, which is running code) is checked as it arrives
//! and kept only if it carries the standard content policy; see `scene.rs`.

use std::collections::BTreeMap;
use std::fs;
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

use crate::model::StoredFile;
use crate::scene;

pub const MAX_DOWNLOAD_BYTES: u64 = 300 * 1024 * 1024;
const MAX_NAME_LENGTH: usize = 120;
const TEMP_SUFFIX: &str = ".part";

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

#[derive(Debug, Clone)]
pub struct Library {
    dir: PathBuf,
}

impl Library {
    /// Creates the folder if needed and clears temporary files a previous run
    /// left behind (for example a download cut short by quitting).
    pub fn open(dir: PathBuf) -> Result<Self, String> {
        fs::create_dir_all(&dir)
            .map_err(|error| format!("The library folder could not be created: {error}."))?;
        if let Ok(entries) = fs::read_dir(&dir) {
            for entry in entries.flatten() {
                let name = entry.file_name();
                let name = name.to_string_lossy();
                if name.starts_with('.') && name.ends_with(TEMP_SUFFIX) {
                    let _ = fs::remove_file(entry.path());
                }
            }
        }
        Ok(Library { dir })
    }

    /// Keep `bytes` under `name`, replacing any file already stored under it.
    pub fn store(&self, name: &str, bytes: &[u8]) -> Result<StoredFile, String> {
        validate_name(name)?;
        if scene::is_scene_name(name) && scene::check_scene(bytes).is_err() {
            // Refused before anything is written.
            return Err(scene::REFUSAL.to_string());
        }
        let temp = self.temp_path(name);
        let written = fs::File::create(&temp).and_then(|mut file| file.write_all(bytes));
        if let Err(error) = written {
            let _ = fs::remove_file(&temp);
            return Err(format!("The file could not be saved: {error}."));
        }
        self.publish(&temp, name)
    }

    /// Fetch `url` straight into the library. When `name` has no extension one
    /// is added from the response's Content-Type; the final name is returned.
    pub fn download(&self, name: &str, url: &str) -> Result<StoredFile, String> {
        validate_name(name)?;
        if !is_http_url(url) {
            return Err("Only http and https addresses can be downloaded.".to_string());
        }

        let mut response = download_agent().get(url).call().map_err(describe_download_error)?;

        let final_name = if has_extension(name) {
            name.to_string()
        } else {
            let content_type = response
                .headers()
                .get("content-type")
                .and_then(|value| value.to_str().ok())
                .unwrap_or("");
            let extension = extension_for_content_type(content_type).ok_or_else(|| {
                if content_type.is_empty() {
                    "The server did not say what kind of file this is, so it was not saved.".to_string()
                } else {
                    format!("Still cannot use files of type \"{content_type}\" as wallpapers.")
                }
            })?;
            format!("{name}{extension}")
        };
        validate_name(&final_name)?;

        let is_scene = scene::is_scene_name(&final_name);
        let cap = if is_scene { scene::MAX_SCENE_BYTES } else { MAX_DOWNLOAD_BYTES };
        if response.body().content_length().is_some_and(|length| length > cap) {
            return Err(if is_scene { scene::REFUSAL.to_string() } else { too_large() });
        }

        let temp = self.temp_path(&final_name);
        let copied = copy_capped(response.body_mut().as_reader(), &temp, cap);
        if let Err(error) = copied {
            let _ = fs::remove_file(&temp);
            return Err(if is_scene && error == too_large() { scene::REFUSAL.to_string() } else { error });
        }
        // Checked while it is still a hidden temporary file: a scene that
        // fails never exists in the library, not even for a moment.
        if is_scene && !scene_file_is_acceptable(&temp) {
            let _ = fs::remove_file(&temp);
            return Err(scene::REFUSAL.to_string());
        }
        self.publish(&temp, &final_name)
    }

    /// Delete the named files. Files that are already gone are ignored; a name
    /// that could never have been stored is refused before anything is deleted.
    pub fn remove(&self, names: &[String]) -> Result<(), String> {
        for name in names {
            validate_name(name)?;
        }
        for name in names {
            match fs::remove_file(self.dir.join(name)) {
                Ok(()) => {}
                Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
                Err(error) => return Err(format!("\"{name}\" could not be removed: {error}.")),
            }
        }
        Ok(())
    }

    /// Look names up after a restart. Only names that exist are returned;
    /// invalid names cannot exist, so they are simply absent.
    pub fn resolve(&self, names: &[String]) -> BTreeMap<String, StoredFile> {
        names
            .iter()
            .filter(|name| validate_name(name).is_ok())
            .filter_map(|name| self.describe(name).map(|file| (name.clone(), file)))
            .collect()
    }

    /// True when `path` is a file directly inside the library folder. Paths
    /// arrive from the UI inside play items; nothing outside the library is
    /// ever put on the desktop or loaded into a live wallpaper window.
    pub fn holds(&self, path: &str) -> bool {
        let path = Path::new(path);
        let name_is_valid = path
            .file_name()
            .and_then(|name| name.to_str())
            .is_some_and(|name| validate_name(name).is_ok());
        name_is_valid && path.parent() == Some(self.dir.as_path()) && path.is_file()
    }

    fn describe(&self, name: &str) -> Option<StoredFile> {
        let path = self.dir.join(name);
        let metadata = fs::metadata(&path).ok()?;
        metadata.is_file().then(|| StoredFile {
            name: name.to_string(),
            path: path.to_string_lossy().into_owned(),
            bytes: metadata.len(),
        })
    }

    /// Hidden (leading dot) so it can never collide with a library name and is
    /// never served to the UI; in the same folder so the rename is atomic.
    fn temp_path(&self, name: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        self.dir.join(format!(".{name}.{}.{unique}{TEMP_SUFFIX}", std::process::id()))
    }

    fn publish(&self, temp: &Path, name: &str) -> Result<StoredFile, String> {
        if let Err(error) = fs::rename(temp, self.dir.join(name)) {
            let _ = fs::remove_file(temp);
            return Err(format!("The file could not be saved: {error}."));
        }
        self.describe(name).ok_or_else(|| "The file was saved but could not be read back.".to_string())
    }
}

/// 1 to 120 characters of `[A-Za-z0-9._-]`, no leading dot, no "..".
/// That rules out path separators, hidden files and climbing out of the folder.
pub fn validate_name(name: &str) -> Result<(), String> {
    let allowed = |byte: u8| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b'-');
    let valid = !name.is_empty()
        && name.len() <= MAX_NAME_LENGTH
        && name.bytes().all(allowed)
        && !name.starts_with('.')
        && !name.contains("..");
    if valid {
        Ok(())
    } else {
        Err("That file name is not allowed: use up to 120 letters, digits, dots, dashes or underscores, not starting with a dot.".to_string())
    }
}

/// The extension (with its dot) for the kinds of file Still can show.
pub fn extension_for_content_type(content_type: &str) -> Option<&'static str> {
    // "image/jpeg; charset=binary" -> "image/jpeg"
    let essence = content_type.split(';').next().unwrap_or("").trim().to_ascii_lowercase();
    match essence.as_str() {
        "image/jpeg" => Some(".jpg"),
        "image/png" => Some(".png"),
        "image/webp" => Some(".webp"),
        "video/mp4" => Some(".mp4"),
        "video/quicktime" => Some(".mov"),
        "video/webm" => Some(".webm"),
        "text/html" => Some(".html"),
        _ => None,
    }
}

/// Decode the URL-encoded file name the UI sends in the `x-file-name` header
/// (JavaScript's `encodeURIComponent`). `None` for malformed escapes or bytes
/// that are not UTF-8.
pub fn decode_header_name(encoded: &str) -> Option<String> {
    let bytes = encoded.as_bytes();
    let mut out = Vec::with_capacity(bytes.len());
    let mut index = 0;
    while index < bytes.len() {
        if bytes[index] == b'%' {
            let high = hex_value(*bytes.get(index + 1)?)?;
            let low = hex_value(*bytes.get(index + 2)?)?;
            out.push(high * 16 + low);
            index += 3;
        } else {
            out.push(bytes[index]);
            index += 1;
        }
    }
    String::from_utf8(out).ok()
}

fn hex_value(byte: u8) -> Option<u8> {
    match byte {
        b'0'..=b'9' => Some(byte - b'0'),
        b'a'..=b'f' => Some(byte - b'a' + 10),
        b'A'..=b'F' => Some(byte - b'A' + 10),
        _ => None,
    }
}

fn has_extension(name: &str) -> bool {
    Path::new(name).extension().is_some_and(|extension| !extension.is_empty())
}

fn is_http_url(url: &str) -> bool {
    let lower = url.trim_start().to_ascii_lowercase();
    lower.starts_with("http://") || lower.starts_with("https://")
}

fn too_large() -> String {
    format!("That file is larger than {} MB, the most Still will download.", MAX_DOWNLOAD_BYTES / (1024 * 1024))
}

fn download_agent() -> ureq::Agent {
    // Redirects are followed by default; the catalog answers 302 to a signed
    // storage address. There is deliberately no total time limit, because a
    // 300 MB video on a slow line is legitimate: a stalled transfer is caught
    // by the per-read limit instead.
    let config = ureq::Agent::config_builder()
        .timeout_resolve(Some(Duration::from_secs(15)))
        .timeout_connect(Some(Duration::from_secs(15)))
        .timeout_recv_response(Some(Duration::from_secs(30)))
        .timeout_per_read(Some(Duration::from_secs(30)))
        .max_redirects(8)
        .build();
    ureq::Agent::new_with_config(config)
}

fn describe_download_error(error: ureq::Error) -> String {
    match error {
        ureq::Error::StatusCode(code) => format!("The download failed: the server answered with status {code}."),
        ureq::Error::Timeout(_) => "The download timed out.".to_string(),
        other => format!("The download failed: {other}."),
    }
}

/// The downloaded scene at `path` passes `scene::check_scene`.
fn scene_file_is_acceptable(path: &Path) -> bool {
    let small_enough = fs::metadata(path).is_ok_and(|metadata| metadata.len() <= scene::MAX_SCENE_BYTES);
    small_enough && fs::read(path).is_ok_and(|bytes| scene::check_scene(&bytes).is_ok())
}

/// Stream `reader` into `destination`, giving up (the caller deletes the file)
/// as soon as more than `cap` bytes have arrived. Servers can lie about or
/// omit Content-Length, so the count is what is trusted.
fn copy_capped(mut reader: impl Read, destination: &Path, cap: u64) -> Result<u64, String> {
    let mut file = fs::File::create(destination)
        .map_err(|error| format!("The file could not be saved: {error}."))?;
    let mut buffer = vec![0u8; 64 * 1024];
    let mut total: u64 = 0;
    loop {
        let read = match reader.read(&mut buffer) {
            Ok(0) => break,
            Ok(read) => read,
            Err(error) if error.kind() == std::io::ErrorKind::Interrupted => continue,
            Err(error) => return Err(format!("The download was interrupted: {error}.")),
        };
        total += read as u64;
        if total > cap {
            return Err(too_large());
        }
        file.write_all(&buffer[..read])
            .map_err(|error| format!("The file could not be saved: {error}."))?;
    }
    Ok(total)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn scratch(label: &str) -> PathBuf {
        let unique = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
        let dir = std::env::temp_dir().join(format!("still-library-test-{}-{label}-{unique}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        dir
    }

    #[test]
    fn ordinary_names_are_accepted() {
        for name in ["a", "a1b2c3.jpg", "Sunset_01-final.webp", "clip.v2.mp4", "scene.html", "A-b_c.D", &"x".repeat(120)] {
            assert!(validate_name(name).is_ok(), "{name:?} should be allowed");
        }
    }

    #[test]
    fn dangerous_or_malformed_names_are_refused() {
        let too_long = "x".repeat(121);
        for name in [
            "",
            ".",
            "..",
            ".hidden",
            ".env",
            "a..b",
            "../escape.jpg",
            "a/b.jpg",
            "a\\b.jpg",
            "/etc/passwd",
            "name with space.jpg",
            "caf\u{e9}.jpg",
            "a:b.jpg",
            "a\0b",
            "a\nb",
            "tilde~.jpg",
            "percent%20.jpg",
            too_long.as_str(),
        ] {
            assert!(validate_name(name).is_err(), "{name:?} should be refused");
        }
    }

    #[test]
    fn content_types_map_to_extensions() {
        for (content_type, extension) in [
            ("image/jpeg", ".jpg"),
            ("image/png", ".png"),
            ("image/webp", ".webp"),
            ("video/mp4", ".mp4"),
            ("video/quicktime", ".mov"),
            ("video/webm", ".webm"),
            ("text/html", ".html"),
            ("text/html; charset=utf-8", ".html"),
            ("IMAGE/JPEG", ".jpg"),
            ("  image/png ;q=1", ".png"),
        ] {
            assert_eq!(extension_for_content_type(content_type), Some(extension), "{content_type}");
        }
        for content_type in ["", "application/octet-stream", "image/gif", "image/heic", "image", "text/plain", "application/json"] {
            assert_eq!(extension_for_content_type(content_type), None, "{content_type}");
        }
    }

    #[test]
    fn every_mapped_extension_makes_a_valid_name() {
        for content_type in ["image/jpeg", "image/png", "image/webp", "video/mp4", "video/quicktime", "video/webm", "text/html"] {
            let name = format!("abc123{}", extension_for_content_type(content_type).unwrap());
            assert!(validate_name(&name).is_ok());
            assert!(has_extension(&name));
        }
        assert!(!has_extension("abc123"));
        assert!(!has_extension("abc123."));
    }

    #[test]
    fn header_names_are_url_decoded() {
        assert_eq!(decode_header_name("a1b2c3.jpg").as_deref(), Some("a1b2c3.jpg"));
        assert_eq!(decode_header_name("my%20photo.jpg").as_deref(), Some("my photo.jpg"));
        assert_eq!(decode_header_name("caf%C3%A9.png").as_deref(), Some("caf\u{e9}.png"));
        assert_eq!(decode_header_name("..%2F..%2Fetc").as_deref(), Some("../../etc"));
        assert_eq!(decode_header_name("bad%2"), None);
        assert_eq!(decode_header_name("bad%zz"), None);
        assert_eq!(decode_header_name("%ff%fe"), None);
        // Decoding never makes an unsafe name acceptable.
        assert!(validate_name(&decode_header_name("..%2F..%2Fetc").unwrap()).is_err());
    }

    #[test]
    fn only_http_and_https_are_downloadable() {
        assert!(is_http_url("https://example.com/a.jpg"));
        assert!(is_http_url("HTTP://example.com/a.jpg"));
        for url in ["file:///etc/passwd", "ftp://example.com/a", "javascript:alert(1)", "example.com/a.jpg", "", "asset://localhost/x"] {
            assert!(!is_http_url(url), "{url}");
        }
        let library = Library::open(scratch("scheme")).unwrap();
        assert!(library.download("a.jpg", "file:///etc/passwd").is_err());
    }

    #[test]
    fn store_replaces_resolve_finds_and_remove_forgets() {
        let dir = scratch("roundtrip");
        let library = Library::open(dir.clone()).unwrap();

        let first = library.store("a1.jpg", b"hello").unwrap();
        assert_eq!(first.name, "a1.jpg");
        assert_eq!(first.bytes, 5);
        assert_eq!(Path::new(&first.path), dir.join("a1.jpg"));

        let replaced = library.store("a1.jpg", b"hello again").unwrap();
        assert_eq!(replaced.bytes, 11);
        assert_eq!(fs::read(dir.join("a1.jpg")).unwrap(), b"hello again");

        let found = library.resolve(&["a1.jpg".into(), "missing.jpg".into(), "../a1.jpg".into()]);
        assert_eq!(found.len(), 1);
        assert_eq!(found["a1.jpg"], replaced);

        assert!(library.holds(&replaced.path));
        assert!(!library.holds(dir.join("missing.jpg").to_str().unwrap()));
        assert!(!library.holds("/etc/hosts"));
        assert!(!library.holds(dir.join("sub").join("a1.jpg").to_str().unwrap()));
        assert!(!library.holds(&format!("{}/../{}/a1.jpg", dir.display(), dir.file_name().unwrap().to_string_lossy())));

        // No temporary files are left behind.
        let leftovers: Vec<_> = fs::read_dir(&dir).unwrap().flatten().map(|entry| entry.file_name()).collect();
        assert_eq!(leftovers.len(), 1);

        library.remove(&["a1.jpg".into(), "never-there.jpg".into()]).unwrap();
        assert!(library.resolve(&["a1.jpg".into()]).is_empty());
        let _ = fs::remove_dir_all(&dir);
    }

    const GOOD_SCENE: &str = "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; base-uri 'none'; form-action 'none'\">\n<title>Quiet Clock</title>\n</head>\n<body><script>draw()</script></body>\n</html>\n";
    /// The policy text scenes carried before the standard: no longer accepted on arrival.
    const OLD_SCENE: &str = "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'\">\n</head>\n<body><script>draw()</script></body>\n</html>\n";

    fn names_in(dir: &Path) -> Vec<String> {
        let mut names: Vec<String> =
            fs::read_dir(dir).unwrap().flatten().map(|entry| entry.file_name().to_string_lossy().into_owned()).collect();
        names.sort();
        names
    }

    /// Answer the next request on a local port with `body`, then stop.
    /// Returns the address to download from.
    fn serve_once(content_type: &'static str, body: Vec<u8>) -> String {
        use std::io::BufRead;
        let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        std::thread::spawn(move || {
            let Ok((stream, _)) = listener.accept() else { return };
            let mut reader = std::io::BufReader::new(&stream);
            let mut line = String::new();
            // Read the request up to its blank line.
            while reader.read_line(&mut line).is_ok_and(|read| read > 2) {
                line.clear();
            }
            let head = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: {content_type}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                body.len()
            );
            let mut stream = &stream;
            let _ = stream.write_all(head.as_bytes());
            let _ = stream.write_all(&body);
        });
        format!("http://{address}/file")
    }

    #[test]
    fn a_stored_scene_must_carry_the_policy() {
        let dir = scratch("scene-store");
        let library = Library::open(dir.clone()).unwrap();

        let kept = library.store("clock.html", GOOD_SCENE.as_bytes()).unwrap();
        assert_eq!(kept.name, "clock.html");

        for (name, body) in [
            ("old.html", OLD_SCENE.as_bytes()),
            ("bare.html", "<script>fetch('https://example.com')</script>".as_bytes()),
            ("picture.html", &[0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, b'J', b'F', b'I', b'F'][..]),
            ("Shouting.HTML", OLD_SCENE.as_bytes()),
            ("short.htm", OLD_SCENE.as_bytes()),
        ] {
            assert_eq!(library.store(name, body).unwrap_err(), scene::REFUSAL, "{name}");
        }
        // Nothing but the good scene exists: no refused file, no temporary file.
        assert_eq!(names_in(&dir), ["clock.html"]);

        // A refused replacement leaves the scene that was there untouched.
        assert!(library.store("clock.html", OLD_SCENE.as_bytes()).is_err());
        assert_eq!(fs::read(dir.join("clock.html")).unwrap(), GOOD_SCENE.as_bytes());

        // Pictures and videos are not judged by this rule, whatever is in them.
        library.store("poster.jpg", OLD_SCENE.as_bytes()).unwrap();
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn a_downloaded_scene_must_carry_the_policy() {
        let dir = scratch("scene-download");
        let library = Library::open(dir.clone()).unwrap();

        // No extension given: the server's type makes it a scene, and it is checked.
        let good = library.download("a1b2c3", &serve_once("text/html; charset=utf-8", GOOD_SCENE.into())).unwrap();
        assert_eq!(good.name, "a1b2c3.html");
        assert_eq!(fs::read(dir.join("a1b2c3.html")).unwrap(), GOOD_SCENE.as_bytes());

        let refused = library.download("d4e5f6", &serve_once("text/html", OLD_SCENE.into())).unwrap_err();
        assert_eq!(refused, scene::REFUSAL);
        // The name decides too, whatever type the server claims.
        let mislabelled = library.download("g7h8i9.html", &serve_once("image/jpeg", OLD_SCENE.into())).unwrap_err();
        assert_eq!(mislabelled, scene::REFUSAL);
        let not_html = library.download("j1k2l3.html", &serve_once("text/html", vec![0xFF, 0xD8, 0xFF, 0xE0])).unwrap_err();
        assert_eq!(not_html, scene::REFUSAL);
        let oversized = vec![b' '; scene::MAX_SCENE_BYTES as usize + 1];
        let too_big = library.download("m4n5o6", &serve_once("text/html", oversized)).unwrap_err();
        assert_eq!(too_big, scene::REFUSAL);

        // Only the good scene is in the folder: nothing refused, nothing half-written.
        assert_eq!(names_in(&dir), ["a1b2c3.html"]);

        // An ordinary picture download is unaffected.
        let picture = library.download("p7q8r9", &serve_once("image/jpeg", vec![0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3])).unwrap();
        assert_eq!(picture.name, "p7q8r9.jpg");
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn remove_refuses_bad_names_before_deleting_anything() {
        let dir = scratch("remove");
        let library = Library::open(dir.clone()).unwrap();
        library.store("keep.jpg", b"x").unwrap();
        assert!(library.remove(&["keep.jpg".into(), "../outside".into()]).is_err());
        assert!(dir.join("keep.jpg").exists());
        assert!(library.store("../outside.jpg", b"x").is_err());
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn opening_clears_stale_temporary_files() {
        let dir = scratch("stale");
        fs::create_dir_all(&dir).unwrap();
        fs::write(dir.join(".a.jpg.1.2.part"), b"half").unwrap();
        fs::write(dir.join("real.jpg"), b"whole").unwrap();
        Library::open(dir.clone()).unwrap();
        assert!(!dir.join(".a.jpg.1.2.part").exists());
        assert!(dir.join("real.jpg").exists());
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn a_stream_over_the_cap_is_cut_off() {
        let dir = scratch("cap");
        fs::create_dir_all(&dir).unwrap();
        let destination = dir.join("out.bin");
        let data = vec![7u8; 200_000];
        assert_eq!(copy_capped(&data[..], &destination, 200_000).unwrap(), 200_000);
        assert_eq!(fs::metadata(&destination).unwrap().len(), 200_000);
        assert!(copy_capped(&data[..], &destination, 199_999).is_err());
        let _ = fs::remove_dir_all(&dir);
    }
}
