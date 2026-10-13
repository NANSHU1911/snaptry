// SnapTry content script — captures a garment image when the user drags it.
// Original code. Runs on all http/https pages (MV3 content_scripts match).
(function () {
  async function urlToDataURL(url) {
    const resp = await fetch(url, { credentials: "omit" });
    const blob = await resp.blob();
    return new Promise((resolve, reject) => {
      const r = new FileReader();
      r.onload = () => resolve(r.result);
      r.onerror = reject;
      r.readAsDataURL(blob);
    });
  }

  function toast(msg) {
    const el = document.createElement("div");
    el.textContent = msg;
    el.style.cssText =
      "position:fixed;z-index:2147483647;right:16px;bottom:16px;background:#111;color:#fff;" +
      "font:13px/1.5 sans-serif;padding:10px 14px;border-radius:10px;opacity:.95;pointer-events:none";
    document.body.appendChild(el);
    setTimeout(() => el.remove(), 2600);
  }

  document.addEventListener("dragstart", async (e) => {
    try {
      const t = e.target;
      let src = null;
      if (t && t.tagName === "IMG") src = t.currentSrc || t.src;
      // Also handle drag of a link wrapping an image
      if (!src && t && t.closest) {
        const img = t.closest("a")?.querySelector("img");
        if (img) src = img.currentSrc || img.src;
      }
      if (!src || src.startsWith("data:")) return;
      const dataURL = await urlToDataURL(src);
      await chrome.storage.local.set({ garmentImage: dataURL, garmentSource: location.href });
      toast("已捕获衣服图，打开 SnapTry 插件开始试穿");
    } catch (err) {
      // CORS-blocked image: fall back to storing the URL; gateway may fetch it server-side.
      try {
        const t = e.target;
        const src = t && t.tagName === "IMG" ? t.currentSrc || t.src : null;
        if (src) {
          await chrome.storage.local.set({ garmentImage: src, garmentSource: location.href });
          toast("已记录衣服图链接（直连失败，已存原链接）");
        }
      } catch (_) { /* ignore */ }
    }
  }, true);
})();
