//! Is the Mac running on its battery?
//!
//! Asked through IOKit's public power-source API rather than by running
//! `pmset`, because a sandboxed App Store build may not launch other programs.

use std::ffi::{c_char, c_void, CStr};

#[link(name = "IOKit", kind = "framework")]
extern "C" {
    fn IOPSCopyPowerSourcesInfo() -> *const c_void;
    fn IOPSGetProvidingPowerSourceType(snapshot: *const c_void) -> *const c_void;
}

#[link(name = "CoreFoundation", kind = "framework")]
extern "C" {
    fn CFRelease(object: *const c_void);
    fn CFStringGetCString(string: *const c_void, buffer: *mut c_char, buffer_size: isize, encoding: u32) -> u8;
}

const CF_STRING_ENCODING_UTF8: u32 = 0x0800_0100;
/// The value of IOKit's `kIOPMBatteryPowerKey`.
const BATTERY_POWER: &str = "Battery Power";

/// The name macOS gives the source currently powering the machine:
/// "AC Power", "Battery Power" or "UPS Power". `None` if it cannot be read.
pub fn providing_source() -> Option<String> {
    // SAFETY: plain C calls. The snapshot follows the Copy rule (we release
    // it); the string follows the Get rule (owned by the system, not released).
    unsafe {
        let snapshot = IOPSCopyPowerSourcesInfo();
        if snapshot.is_null() {
            return None;
        }
        let source = IOPSGetProvidingPowerSourceType(snapshot);
        let mut buffer = [0 as c_char; 64];
        let name = if !source.is_null()
            && CFStringGetCString(source, buffer.as_mut_ptr(), buffer.len() as isize, CF_STRING_ENCODING_UTF8) != 0
        {
            Some(CStr::from_ptr(buffer.as_ptr()).to_string_lossy().into_owned())
        } else {
            None
        };
        CFRelease(snapshot);
        name
    }
}

/// True only when a battery is what is powering the Mac. A desktop Mac, or a
/// laptop on its charger, is not "on battery"; neither is an unknown state,
/// so a failed reading never freezes live wallpapers.
pub fn on_battery() -> bool {
    providing_source().as_deref() == Some(BATTERY_POWER)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reading_the_power_source_is_safe_and_plausible() {
        // Read-only. On any Mac this is one of the three documented names.
        if let Some(source) = providing_source() {
            assert!(
                ["AC Power", "Battery Power", "UPS Power"].contains(&source.as_str()),
                "unexpected power source {source:?}"
            );
        }
        let _ = on_battery();
    }
}
