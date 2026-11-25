// A classic script in the document head applies the saved palette before first paint.
(() => {
  const root = document.documentElement;
  const system = window.matchMedia("(prefers-color-scheme: dark)");
  const tint = document.querySelector('meta[name="theme-color"]');
  const read = () => {
    try { return localStorage.getItem("theme"); } catch { return null; }
  };
  function apply(value) {
    const choice = value === "light" || value === "dark" ? value : "system";
    const dark = choice === "dark" || (choice === "system" && system.matches);
    const scheme = dark ? "dark" : "light";
    root.dataset.appearance = choice;
    root.dataset.theme = scheme;
    root.classList.toggle("dark", dark);
    root.classList.toggle("light", !dark);
    root.style.colorScheme = scheme;
    tint.content = dark ? "#10141a" : "#f0f3f8";
  }
  apply(read());
  system.addEventListener("change", () => apply(root.dataset.appearance));
  window.addEventListener("storage", event => {
    if (event.key === "theme" || event.key === null) apply(read());
  });
  window.addEventListener("spillup:appearance", event => apply(event.detail));
})();
