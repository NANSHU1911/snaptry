// SnapTry background service worker (MV3) — talks to the user's own Go gateway.
// Original code. The extension never calls third-party try-on APIs directly.
const POLL_INTERVAL_MS = 3000;
const POLL_TIMEOUT_MS = 5 * 60 * 1000;

async function postTryon(gatewayUrl, gatewayKey, personImage, garmentImage) {
  const resp = await fetch(gatewayUrl.replace(/\/$/, "") + "/api/tryon", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-API-Key": gatewayKey,
    },
    body: JSON.stringify({
      person_image: personImage,
      garment_image: garmentImage,
      category: "auto",
    }),
  });
  if (!resp.ok) {
    const text = await resp.text().catch(() => "");
    throw new Error("网关返回 " + resp.status + (text ? ": " + text.slice(0, 200) : ""));
  }
  return resp.json(); // { task_id, status }
}

async function pollResult(gatewayUrl, gatewayKey, taskId) {
  const base = gatewayUrl.replace(/\/$/, "");
  const deadline = Date.now() + POLL_TIMEOUT_MS;
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, POLL_INTERVAL_MS));
    const resp = await fetch(base + "/api/tryon/" + encodeURIComponent(taskId), {
      headers: { "X-API-Key": gatewayKey },
    });
    if (!resp.ok) throw new Error("查询任务失败: HTTP " + resp.status);
    const data = await resp.json(); // { task_id, status, result_url?, error? }
    if (data.status === "done") return data.result_url;
    if (data.status === "failed") throw new Error(data.error || "远端试穿失败");
    // else: queued / running -> keep polling
  }
  throw new Error("等待超时（5 分钟），请稍后重试");
}

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg.type !== "TRYON") return false;
  (async () => {
    try {
      const { task_id } = await postTryon(msg.gatewayUrl, msg.gatewayKey, msg.personImage, msg.garmentImage);
      const resultUrl = await pollResult(msg.gatewayUrl, msg.gatewayKey, task_id);
      sendResponse({ ok: true, resultUrl });
    } catch (err) {
      sendResponse({ ok: false, error: err.message });
    }
  })();
  return true; // keep the message channel open for async response
});
