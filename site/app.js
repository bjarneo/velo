(() => {
  "use strict";

  window.veloReady = true;
  const root = document.documentElement;
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const css = (name) => getComputedStyle(root).getPropertyValue(name).trim();

  /* themes: the same 8 palettes as velo */
  const THEMES = [
    ["tokyo-night", "tokyo night"], ["catppuccin", "catppuccin"], ["gruvbox", "gruvbox"], ["nord", "nord"],
    ["everforest", "everforest"], ["kanagawa", "kanagawa"], ["rose-pine", "rosé pine"], ["matte-black", "matte black"],
  ];
  const themeName = document.getElementById("theme-name");
  const themeMeta = document.querySelector('meta[name="theme-color"]');

  function applyTheme(id, save) {
    root.dataset.theme = id;
    const name = (THEMES.find((t) => t[0] === id) || THEMES[0])[1];
    themeName.textContent = name + " · press t";
    themeMeta.setAttribute("content", css("--bg"));
    if (save) {
      try { localStorage.setItem("velo-theme", id); } catch (e) { /* storage is optional */ }
    }
    drawField();
  }

  function cycleTheme() {
    const i = THEMES.findIndex((t) => t[0] === root.dataset.theme);
    applyTheme(THEMES[(i + 1) % THEMES.length][0], true);
  }

  /* the braille field: velo's timeline plot, drawn as dots behind the window */
  const canvas = document.querySelector(".field");
  const ctx = canvas.getContext("2d");
  const CW = 10, CH = 21;

  function drawField() {
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const w = innerWidth, h = innerHeight;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);

    const cols = Math.ceil(w / CW) * 2, rows = Math.ceil(h / CH) * 4;
    const plotH = rows * 0.42, split = Math.round(cols * 8 / 15);
    const dl = css("--cyan"), ul = css("--mag"), grid = css("--line");
    let seed = 7;
    const rand = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
    let noise = 0;
    for (let x = 0; x < cols; x++) {
      const down = x < split;
      const t = down ? x / split : (x - split) / (cols - split);
      noise = noise * 0.9 + (rand() - 0.5) * 0.06;
      const level = Math.max(0.05, (1 - Math.exp(-t * (down ? 9 : 7))) * (down ? 0.86 : 0.8) + noise);
      const top = Math.round(level * plotH);
      for (let yb = 1; yb <= top; yb++) {
        const edge = yb >= top - 1;
        if (!edge && (x + yb) % 2 !== 0) continue;
        ctx.globalAlpha = edge ? 0.45 : 0.18;
        ctx.fillStyle = down ? dl : ul;
        dot(x, rows - yb);
      }
      if (x % 4 === 0) {
        ctx.globalAlpha = 0.5;
        ctx.fillStyle = grid;
        dot(x, rows - Math.round(plotH) - 2);
        dot(x, rows - 1);
      }
    }
    ctx.globalAlpha = 1;
  }

  function dot(dx, dy) {
    // a braille cell is 2 dots wide and 4 dots high
    const x = (Math.floor(dx / 2) * CW) + (dx % 2 === 0 ? CW * 0.28 : CW * 0.72);
    const y = (Math.floor(dy / 4) * CH) + CH * ((dy % 4) * 2 + 1) / 8;
    ctx.beginPath();
    ctx.arc(x, y, 1.45, 0, Math.PI * 2);
    ctx.fill();
  }

  let resizeTimer;
  addEventListener("resize", () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(drawField, 120); });

  /* motion: each command types itself once, then its output follows line by line */
  const state = document.getElementById("state");
  const wait = (ms) => new Promise((r) => setTimeout(r, ms));

  // Split each output block into lines, so that the lines can appear one by one.
  document.querySelectorAll("pre.out").forEach((pre) => {
    pre.innerHTML = pre.innerHTML.split("\n").map((l) => '<span class="ln">' + (l || " ") + "</span>").join("");
    pre.classList.add("split");
  });

  async function type(code) {
    const text = code.textContent;
    const step = Math.max(6, Math.min(24, 700 / text.length));
    code.textContent = "";
    code.classList.add("typing");
    for (let n = 1; n <= text.length; n++) {
      code.textContent = text.slice(0, n);
      await wait(step);
    }
    await wait(140);
    code.classList.remove("typing");
    code.classList.add("typed");
  }

  async function play(beat) {
    for (const el of beat.querySelectorAll("code[data-type], pre.out, .reveal")) {
      if (el.matches("code[data-type]")) {
        await type(el);
      } else if (el.matches("pre.out")) {
        const lines = el.querySelectorAll(".ln");
        for (const ln of lines) {
          ln.classList.add("shown");
          await wait(lines.length > 8 ? 26 : 70);
        }
      } else {
        el.classList.add("shown");
        await wait(90);
      }
    }
    if (beat.id === "run") state.textContent = "✓ COMPLETE";
  }

  if (root.classList.contains("motion")) {
    const io = new IntersectionObserver((entries) => {
      for (const e of entries) {
        if (!e.isIntersecting) continue;
        io.unobserve(e.target);
        play(e.target);
      }
    }, { rootMargin: "0px 0px -18% 0px" });
    document.querySelectorAll("[data-beat]").forEach((b) => io.observe(b));
  }

  /* the keys: the page answers velo's keys */
  const SCREENS = {
    enter: ["assets/screenshot-complete.png", "real run", "⏎ runs the test. This run used Wi-Fi in foot on Omarchy.",
      "velo after a finished test: download 495.8 Mbps, upload 495.6 Mbps, grade C"],
    s: ["assets/servers.png", "--demo", "s opens the server list. velo measures the latency to the 10 nearest servers and selects the fastest one. Rendered with --demo.",
      "The server list with sponsor, location, distance, and estimated round-trip time"],
    h: ["assets/history.png", "--demo", "h shows the history: the run count, the averages, the best download, and each run with its grade. Rendered with --demo.",
      "The history panel with run statistics and a table of runs"],
    t: ["assets/themes.png", "--demo", "t cycles the theme. On Omarchy, velo uses the active theme and follows each change. This page changed its theme too.",
      "velo in the Tokyo Night, Gruvbox, Nord, and Rosé Pine themes"],
    "?": ["assets/keys.png", "--demo", "? shows every key. Press esc to close a panel. Rendered with --demo.",
      "The keys panel of velo"],
    q: [null, null, "q quits velo. velo prints the last result in your shell: velo: session closed · last run ↓ 495.8 ↑ 495.6 Mbps · 4.0 ms · C", null],
  };
  const img = document.getElementById("screen-img");
  const cap = document.getElementById("screen-cap");
  const tag = document.getElementById("screen-tag");
  const keysBeat = document.getElementById("keys");
  const panelButtons = [...document.querySelectorAll(".keylist button")];

  function show(key) {
    const [src, label, text, alt] = SCREENS[key];
    panelButtons.forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.key === key)));
    cap.textContent = text;
    if (!src || img.getAttribute("src") === src) return;
    tag.textContent = label;
    img.classList.add("swap");
    const next = new Image();
    next.onload = next.onerror = () => {
      img.src = src;
      img.alt = alt;
      requestAnimationFrame(() => img.classList.remove("swap"));
    };
    next.src = src;
  }

  function inView(el) {
    const r = el.getBoundingClientRect();
    return r.top < innerHeight * 0.7 && r.bottom > innerHeight * 0.3;
  }

  function press(key, fromPanel) {
    if (key === "t") cycleTheme();
    show(key);
    if (fromPanel) return;
    const target = key === "q" ? document.getElementById("close") : key === "t" ? null : keysBeat;
    if (target && !inView(target)) target.scrollIntoView({ behavior: reduce ? "auto" : "smooth", block: "center" });
  }

  panelButtons.forEach((b) => b.addEventListener("click", () => press(b.dataset.key, true)));
  document.querySelectorAll(".foot .keys button").forEach((b) => b.addEventListener("click", () => press(b.dataset.key, false)));

  // Single-key shortcuts can be turned off (WCAG 2.1.4). The choice is saved.
  const hotkeys = document.getElementById("hotkeys");
  let keysOn = true;
  try { keysOn = localStorage.getItem("velo-keys") !== "off"; } catch (e) { /* storage is optional */ }
  const syncKeys = () => {
    hotkeys.setAttribute("aria-pressed", String(keysOn));
    hotkeys.textContent = keysOn ? "keys on" : "keys off";
  };
  hotkeys.addEventListener("click", () => {
    keysOn = !keysOn;
    try { localStorage.setItem("velo-keys", keysOn ? "on" : "off"); } catch (e) { /* storage is optional */ }
    syncKeys();
  });
  syncKeys();

  addEventListener("keydown", (e) => {
    if (!keysOn || e.metaKey || e.ctrlKey || e.altKey || e.defaultPrevented) return;
    const t = e.target;
    if (t.closest && t.closest("input, textarea, select, [contenteditable]")) return;
    let key = e.key;
    if (key === "Enter") {
      if (t.closest && t.closest("a, button")) return;
      key = "enter";
    }
    if (key === "Escape") key = "enter";
    if (!(key in SCREENS)) return;
    e.preventDefault();
    press(key, false);
  });

  // load the other screens after the page settles, so a key press swaps at once
  addEventListener("load", () => setTimeout(() => {
    Object.values(SCREENS).forEach(([src]) => { if (src) new Image().src = src; });
  }, 1500));

  /* copy keycaps */
  document.querySelectorAll("[data-copy]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      let ok = true;
      try { await navigator.clipboard.writeText(btn.dataset.copy); } catch (e) { ok = false; }
      btn.textContent = ok ? "copied" : "select and copy";
      if (ok) btn.dataset.copied = "";
      setTimeout(() => { btn.textContent = "copy"; delete btn.dataset.copied; }, 1600);
    });
  });

  /* the recording: no autoplay with reduced motion, and a pause control */
  const video = document.querySelector(".recording video");
  const toggle = document.getElementById("play");
  if (reduce) { video.removeAttribute("autoplay"); video.pause(); }
  const sync = () => { toggle.textContent = video.paused ? "play" : "pause"; };
  toggle.addEventListener("click", () => { if (video.paused) video.play(); else video.pause(); });
  video.addEventListener("play", sync);
  video.addEventListener("pause", sync);
  sync();

  applyTheme(root.dataset.theme || "tokyo-night", false);
})();
