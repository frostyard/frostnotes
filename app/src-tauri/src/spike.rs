//! M0 spikes run inside the real shell (docs/plans/0001-m0-foundations.md,
//! Phase 5). With FROSTNOTES_SPIKE=cm6 the page runs the CodeMirror spike
//! instead of the app (src/spike), hands its results to `spike_report`,
//! which prints them on stdout as one JSON line, and the app exits. With
//! FROSTNOTES_SPIKE=cm6:manual it only shows the spike, for checks by hand
//! such as IME input. M3's exit evidence (docs/plans/0005-m3-desktop-app.md,
//! Phase 7) drives the app itself the same way; its page asks the harness
//! for key presses with `spike_say`, one JSON line each, before it reports.

use std::io::Write;
use std::time::Duration;

use tauri::{AppHandle, State};

/// How long a spike may run before the app gives up and exits with status 2.
const TIMEOUT: Duration = Duration::from_secs(600);

/// The spike FROSTNOTES_SPIKE names, if any.
#[derive(Default)]
pub struct Spike(pub Option<String>);

impl Spike {
    pub fn from_env() -> Self {
        Spike(std::env::var("FROSTNOTES_SPIKE").ok().filter(|v| !v.is_empty()))
    }

    pub fn active(&self) -> bool {
        self.0.is_some()
    }

    /// A manual spike waits for the person at the window: no timeout.
    pub fn manual(&self) -> bool {
        self.0.as_deref().is_some_and(|v| v.ends_with(":manual"))
    }
}

/// The spike the page should run, or none for the app.
#[tauri::command]
pub fn spike_mode(spike: State<'_, Spike>) -> Option<String> {
    spike.0.clone()
}

/// Print the spike's results and exit. Only a spike run may call it.
#[tauri::command]
pub fn spike_report(app: AppHandle, spike: State<'_, Spike>, report: String) -> Result<(), String> {
    if !spike.active() {
        return Err("no spike is running".into());
    }
    let line = report_line(report)?;
    let mut out = std::io::stdout().lock();
    out.write_all(line.as_bytes()).and_then(|()| out.flush()).map_err(|e| format!("print the report: {e}"))?;
    app.exit(0);
    Ok(())
}

/// Print one line for the harness driving the spike, and go on. Only a
/// spike run may call it.
#[tauri::command]
pub fn spike_say(spike: State<'_, Spike>, line: String) -> Result<(), String> {
    if !spike.active() {
        return Err("no spike is running".into());
    }
    let line = report_line(line)?;
    let mut out = std::io::stdout().lock();
    out.write_all(line.as_bytes()).and_then(|()| out.flush()).map_err(|e| format!("print the line: {e}"))
}

/// The report as printed: one line.
fn report_line(report: String) -> Result<String, String> {
    if report.contains('\n') {
        return Err("the report must be one line".into());
    }
    Ok(report + "\n")
}

/// Exit with status 2 if the spike has not reported within TIMEOUT.
pub fn watchdog(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(TIMEOUT).await;
        eprintln!("frostnotes: the spike did not report within {}s", TIMEOUT.as_secs());
        app.exit(2);
    });
}

#[cfg(test)]
mod tests {
    use super::{Spike, report_line};

    #[test]
    fn only_manual_spikes_wait() {
        assert!(Spike(Some("cm6:manual".into())).manual());
        assert!(!Spike(Some("cm6".into())).manual());
        assert!(!Spike(None).manual());
    }

    #[test]
    fn reports_are_one_line() {
        assert_eq!(report_line(r#"{"spike":"cm6"}"#.into()), Ok("{\"spike\":\"cm6\"}\n".into()));
        assert!(report_line("{}\n{}".into()).is_err());
    }
}
