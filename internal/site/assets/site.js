(function () {
  "use strict";

  // The product site at /stageproof/ stores the theme under the same key.
  var THEME_KEY = "stageproof-theme";
  var root = document.documentElement;

  try {
    if (localStorage.getItem(THEME_KEY) === "light") root.setAttribute("data-theme", "light");
  } catch (e) {}

  function themeName() {
    return root.getAttribute("data-theme") === "light" ? "light" : "dark";
  }

  function paintTheme(theme) {
    if (theme === "light") root.setAttribute("data-theme", "light");
    else root.removeAttribute("data-theme");
    var light = theme === "light";
    var btn = document.getElementById("theme-toggle");
    if (btn) {
      btn.setAttribute("aria-pressed", light ? "true" : "false");
      btn.setAttribute("aria-label", light ? "Switch to dark theme" : "Switch to light theme");
    }
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute("content", light ? "#F4F6F8" : "#06080D");
  }

  function wire() {
    paintTheme(themeName());
    var themeBtn = document.getElementById("theme-toggle");
    if (themeBtn) {
      themeBtn.addEventListener("click", function () {
        var next = themeName() === "light" ? "dark" : "light";
        try { localStorage.setItem(THEME_KEY, next); } catch (e) {}
        paintTheme(next);
      });
    }
    var navToggle = document.getElementById("nav-toggle");
    var navLinks = document.getElementById("nav-links");
    if (navToggle && navLinks) {
      navToggle.addEventListener("click", function () {
        var open = navLinks.classList.toggle("is-open");
        navToggle.setAttribute("aria-expanded", open ? "true" : "false");
      });
    }
  }

  window.addEventListener("storage", function (event) {
    if (event.key === THEME_KEY && (event.newValue === "light" || event.newValue === "dark")) {
      paintTheme(event.newValue);
    }
  });

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", wire);
  else wire();
})();
