(() => {
  const storageKey = "tailor-theme";
  const root = document.documentElement;
  const systemTheme = window.matchMedia("(prefers-color-scheme: dark)");
  let preference = "system";

  try {
    const saved = localStorage.getItem(storageKey);
    if (saved === "light" || saved === "dark") preference = saved;
  } catch {
    // Theme selection still works when browser storage is unavailable.
  }

  function applyTheme() {
    if (preference === "system") root.removeAttribute("data-theme");
    else root.setAttribute("data-theme", preference);
  }

  // Apply a saved choice before stylesheets load to avoid a theme flash.
  applyTheme();

  document.addEventListener("DOMContentLoaded", () => {
    const year = document.getElementById("copyright-year");
    if (year) year.textContent = new Date().getFullYear();

    const toggle = document.getElementById("theme-toggle");
    if (!toggle) return;

    function currentTheme() {
      return preference === "system" ? (systemTheme.matches ? "dark" : "light") : preference;
    }

    function updateControls() {
      const current = currentTheme();
      const next = current === "dark" ? "light" : "dark";
      const label = `Theme: ${preference === "system" ? `system (${current})` : current}. Switch to ${next} mode`;
      toggle.setAttribute("aria-label", label);
      toggle.title = label;
      document.getElementById("theme-sun").toggleAttribute("hidden", current !== "light");
      document.getElementById("theme-moon").toggleAttribute("hidden", current !== "dark");
    }

    function chooseTheme(choice) {
      preference = choice;
      applyTheme();
      updateControls();
      try {
        localStorage.setItem(storageKey, preference);
      } catch {
        // Keep the current choice for this page when storage is unavailable.
      }
    }

    updateControls();
    toggle.hidden = false;
    toggle.addEventListener("click", () => chooseTheme(currentTheme() === "dark" ? "light" : "dark"));
    systemTheme.addEventListener("change", updateControls);
  });
})();
