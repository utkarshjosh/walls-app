//! The wire contract with the React UI.
//!
//! Mirrors `apps/web/src/platform/types.ts` field for field. Everything is
//! camelCase on the wire; change the two files together.

use serde::{Deserialize, Deserializer, Serialize};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DisplayInfo {
    /// CGDirectDisplayID as a string; stable while the display stays connected.
    pub id: String,
    pub name: String,
    /// Points, origin at the top-left of the primary display, y grows downward.
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
    /// Pixels per point.
    pub scale: f64,
    pub primary: bool,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum WallpaperKind {
    Image,
    Video,
    Scene,
}

impl WallpaperKind {
    /// Video and scene wallpapers play in a desktop-level window.
    pub fn is_live(self) -> bool {
        !matches!(self, WallpaperKind::Image)
    }
}

/// A library file. The UI derives the loadable URL itself (`convertFileSrc`).
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredFile {
    pub name: String,
    pub path: String,
    pub bytes: u64,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PlayItem {
    pub id: String,
    pub kind: WallpaperKind,
    pub path: String,
    /// Pixels of the original, 0 when unknown. Used to judge whether the
    /// wallpaper is large enough for a display (see `fit.rs`). Floats because
    /// the UI sends JavaScript numbers; absent in state saved by older builds.
    #[serde(default, deserialize_with = "number_or_zero")]
    pub width: f64,
    #[serde(default, deserialize_with = "number_or_zero")]
    pub height: f64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub poster_path: Option<String>,
}

/// A missing or `null` size means "unknown", not a malformed item.
fn number_or_zero<'de, D: Deserializer<'de>>(deserializer: D) -> Result<f64, D::Error> {
    Ok(Option::<f64>::deserialize(deserializer)?.unwrap_or(0.0))
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum DisplayMode {
    Same,
    Span,
    Each,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApplyRequest {
    pub mode: DisplayMode,
    pub items: Vec<PlayItem>,
    #[serde(default)]
    pub display_id: Option<String>,
}

/// `minutes` stays a float because the UI sends a JavaScript number; whether
/// it is a usable whole number is decided where the schedule is evaluated, so
/// a bad value pauses the timer instead of failing the whole sync.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum Schedule {
    Interval { minutes: f64 },
    Daily { time: String },
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RotationPlan {
    pub items: Vec<PlayItem>,
    pub shuffle: bool,
    pub schedule: Schedule,
    pub paused: bool,
    pub display_mode: DisplayMode,
    pub pause_live_on_battery: bool,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CurrentEntry {
    /// `null` when every display shows the same wallpaper.
    pub display_id: Option<String>,
    pub item_id: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RotationStatus {
    pub paused: bool,
    pub current: Vec<CurrentEntry>,
    /// Epoch milliseconds, or `null` when paused or there is nothing to rotate.
    pub next_change_at: Option<i64>,
    pub can_go_previous: bool,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum RotationCommand {
    Next,
    Previous,
    Pause,
    Resume,
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn plan_reads_the_json_the_ui_sends() {
        let plan: RotationPlan = serde_json::from_value(json!({
            "items": [
                { "id": "a", "kind": "image", "path": "/lib/a.jpg" },
                { "id": "b", "kind": "video", "path": "/lib/b.mp4", "posterPath": "/lib/b.jpg" },
                { "id": "c", "kind": "scene", "path": "/lib/c.html" }
            ],
            "shuffle": true,
            "schedule": { "type": "interval", "minutes": 180 },
            "paused": false,
            "displayMode": "each",
            "pauseLiveOnBattery": true
        }))
        .expect("plan should deserialize");
        assert_eq!(plan.items.len(), 3);
        assert_eq!(plan.items[1].kind, WallpaperKind::Video);
        assert_eq!(plan.items[1].poster_path.as_deref(), Some("/lib/b.jpg"));
        assert_eq!(plan.items[0].poster_path, None);
        // Items saved or sent without a size are "unknown", not an error.
        assert_eq!((plan.items[0].width, plan.items[0].height), (0.0, 0.0));
        assert_eq!(plan.schedule, Schedule::Interval { minutes: 180.0 });
        assert_eq!(plan.display_mode, DisplayMode::Each);
        assert!(plan.pause_live_on_battery);
    }

    #[test]
    fn items_carry_their_pixel_size() {
        let item: PlayItem = serde_json::from_value(json!({
            "id": "a", "kind": "image", "path": "/lib/a.jpg", "width": 3840, "height": 2400
        }))
        .unwrap();
        assert_eq!((item.width, item.height), (3840.0, 2400.0));
        let unknown: PlayItem = serde_json::from_value(json!({
            "id": "a", "kind": "video", "path": "/lib/a.mp4", "width": null, "height": 0
        }))
        .unwrap();
        assert_eq!((unknown.width, unknown.height), (0.0, 0.0));
        let written = serde_json::to_value(&item).unwrap();
        assert_eq!(written["width"], json!(3840.0));
        assert_eq!(written["height"], json!(2400.0));
    }

    #[test]
    fn daily_schedule_is_tagged_on_type() {
        let schedule: Schedule =
            serde_json::from_value(json!({ "type": "daily", "time": "09:30" })).unwrap();
        assert_eq!(schedule, Schedule::Daily { time: "09:30".into() });
        assert_eq!(
            serde_json::to_value(&schedule).unwrap(),
            json!({ "type": "daily", "time": "09:30" })
        );
    }

    #[test]
    fn apply_request_display_id_is_optional() {
        let request: ApplyRequest = serde_json::from_value(json!({
            "mode": "same",
            "items": [{ "id": "a", "kind": "image", "path": "/lib/a.jpg" }]
        }))
        .unwrap();
        assert_eq!(request.display_id, None);
        let request: ApplyRequest = serde_json::from_value(json!({
            "mode": "same",
            "items": [],
            "displayId": "69734662"
        }))
        .unwrap();
        assert_eq!(request.display_id.as_deref(), Some("69734662"));
    }

    #[test]
    fn status_writes_the_json_the_ui_expects() {
        let status = RotationStatus {
            paused: false,
            current: vec![CurrentEntry { display_id: None, item_id: "a".into() }],
            next_change_at: Some(1_800_000_000_000),
            can_go_previous: true,
        };
        assert_eq!(
            serde_json::to_value(&status).unwrap(),
            json!({
                "paused": false,
                "current": [{ "displayId": null, "itemId": "a" }],
                "nextChangeAt": 1_800_000_000_000_i64,
                "canGoPrevious": true
            })
        );
        let empty = serde_json::to_value(RotationStatus::default()).unwrap();
        assert_eq!(empty["nextChangeAt"], serde_json::Value::Null);
        assert_eq!(empty["current"], json!([]));
    }

    #[test]
    fn display_and_file_use_camel_case() {
        let display = DisplayInfo {
            id: "1".into(),
            name: "Built-in Retina Display".into(),
            x: 0.0,
            y: 0.0,
            width: 1512.0,
            height: 982.0,
            scale: 2.0,
            primary: true,
        };
        let value = serde_json::to_value(&display).unwrap();
        assert_eq!(value["width"], json!(1512.0));
        assert_eq!(value["primary"], json!(true));
        let file = StoredFile { name: "a.jpg".into(), path: "/lib/a.jpg".into(), bytes: 12 };
        assert_eq!(
            serde_json::to_value(&file).unwrap(),
            json!({ "name": "a.jpg", "path": "/lib/a.jpg", "bytes": 12 })
        );
    }

    #[test]
    fn commands_are_lowercase_words() {
        for (text, command) in [
            ("next", RotationCommand::Next),
            ("previous", RotationCommand::Previous),
            ("pause", RotationCommand::Pause),
            ("resume", RotationCommand::Resume),
        ] {
            let parsed: RotationCommand = serde_json::from_value(json!(text)).unwrap();
            assert_eq!(parsed, command);
        }
    }
}
