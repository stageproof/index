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
    filter("q", "data-chart", "q-count", "charts");
    filter("steps", "data-step", null, "upgrades");
    var copies = document.querySelectorAll("[data-copy-target]");
    for (var i = 0; i < copies.length; i++) wireCopy(copies[i]);
  }

  // filter hides the rows whose attr does not contain every typed word.
  function filter(inputId, attr, countId, noun) {
    var input = document.getElementById(inputId);
    if (!input) return;
    var rows = document.querySelectorAll("[" + attr + "]");
    var count = countId ? document.getElementById(countId) : null;
    input.addEventListener("input", function () {
      var words = input.value.toLowerCase().split(/\s+/).filter(Boolean);
      var shown = 0;
      for (var i = 0; i < rows.length; i++) {
        var hay = rows[i].getAttribute(attr).toLowerCase();
        var hit = words.every(function (w) { return hay.indexOf(w) !== -1; });
        rows[i].hidden = !hit;
        if (hit) shown++;
      }
      if (count) count.textContent = words.length ? shown + " of " + rows.length + " " + noun : "";
    });
  }

  function wireCopy(btn) {
    var target = document.querySelector(btn.getAttribute("data-copy-target"));
    if (!target || !navigator.clipboard) return;
    btn.addEventListener("click", function () {
      navigator.clipboard.writeText(target.textContent).then(function () {
        btn.textContent = "Copied";
        setTimeout(function () { btn.textContent = "Copy"; }, 1500);
      }).catch(function () {});
    });
  }

  window.addEventListener("storage", function (event) {
    if (event.key === THEME_KEY && (event.newValue === "light" || event.newValue === "dark")) {
      paintTheme(event.newValue);
    }
  });

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", wire);
  else wire();
})();
