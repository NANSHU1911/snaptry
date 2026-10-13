// SnapTry popup logic — original code.
const $ = (id) => document.getElementById(id);
const state = { garment: null, person: null };

function fileToDataURL(file) {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(r.result);
    r.onerror = reject;
    r.readAsDataURL(file);
  });
}

function downscale(dataURL, maxDim = 1024) {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => {
      const scale = Math.min(1, maxDim / Math.max(img.width, img.height));
      if (scale >= 1) return resolve(dataURL);
      const c = document.createElement("canvas");
      c.width = Math.round(img.width * scale);
      c.height = Math.round(img.height * scale);
      c.getContext("2d").drawImage(img, 0, 0, c.width, c.height);
      resolve(c.toDataURL("image/jpeg", 0.85));
    };
    img.onerror = () => resolve(dataURL);
    img.src = dataURL;
  });
}

function refreshButton() {
  const ok = state.garment && state.person && $("consentBox").checked;
  $("btnTryon").disabled = !ok;
}

function setStatus(text, isError = false) {
  const el = $("status");
  el.textContent = text;
  el.classList.toggle("error", isError);
}

async function init() {
  const s = await chrome.storage.local.get(["gatewayUrl", "gatewayKey", "consent", "garmentImage"]);
  if (s.gatewayUrl) $("gatewayUrl").value = s.gatewayUrl;
  if (s.gatewayKey) $("gatewayKey").value = s.gatewayKey;
  if (s.consent) $("consentBox").checked = true;
  if (s.garmentImage) {
    state.garment = s.garmentImage;
    $("garmentPreview").src = s.garmentImage;
  }
  refreshButton();
}

$("btnUploadGarment").onclick = () => $("fileGarment").click();
$("btnUploadPerson").onclick = () => $("filePerson").click();

$("fileGarment").onchange = async (e) => {
  const f = e.target.files[0];
  if (!f) return;
  state.garment = await downscale(await fileToDataURL(f));
  $("garmentPreview").src = state.garment;
  chrome.storage.local.set({ garmentImage: state.garment });
  refreshButton();
};

$("filePerson").onchange = async (e) => {
  const f = e.target.files[0];
  if (!f) return;
  state.person = await downscale(await fileToDataURL(f));
  $("personPreview").src = state.person;
  refreshButton();
};

let stream = null;
$("btnCamera").onclick = async () => {
  try {
    stream = await navigator.mediaDevices.getUserMedia({ video: { width: 720 } });
    $("cameraVideo").srcObject = stream;
    $("cameraBox").classList.remove("hidden");
  } catch (err) {
    setStatus("无法打开摄像头：" + err.message, true);
  }
};
$("btnCameraClose").onclick = () => {
  if (stream) stream.getTracks().forEach((t) => t.stop());
  stream = null;
  $("cameraBox").classList.add("hidden");
};
$("btnShoot").onclick = async () => {
  const v = $("cameraVideo");
  const c = $("cameraCanvas");
  c.width = v.videoWidth; c.height = v.videoHeight;
  c.getContext("2d").drawImage(v, 0, 0);
  state.person = await downscale(c.toDataURL("image/jpeg", 0.9));
  $("personPreview").src = state.person;
  $("btnCameraClose").onclick();
  refreshButton();
};

$("consentBox").onchange = (e) => {
  chrome.storage.local.set({ consent: e.target.checked });
  refreshButton();
};

$("btnSaveSettings").onclick = async () => {
  await chrome.storage.local.set({
    gatewayUrl: $("gatewayUrl").value.trim(),
    gatewayKey: $("gatewayKey").value.trim(),
  });
  setStatus("网关设置已保存");
};

$("btnTryon").onclick = async () => {
  const gatewayUrl = $("gatewayUrl").value.trim();
  const gatewayKey = $("gatewayKey").value.trim();
  if (!gatewayUrl) { setStatus("请先在网关设置里填写网关地址", true); return; }
  setStatus("已提交，AI 正在试穿（约 10~60 秒）…");
  $("btnTryon").disabled = true;
  $("resultCard").classList.add("hidden");
  try {
    const resp = await chrome.runtime.sendMessage({
      type: "TRYON",
      gatewayUrl, gatewayKey,
      personImage: state.person,
      garmentImage: state.garment,
    });
    if (resp && resp.ok) {
      $("resultImg").src = resp.resultUrl;
      $("btnDownload").href = resp.resultUrl;
      $("resultCard").classList.remove("hidden");
      setStatus("试穿完成");
    } else {
      setStatus("试穿失败：" + ((resp && resp.error) || "未知错误"), true);
    }
  } catch (err) {
    setStatus("试穿失败：" + err.message, true);
  }
  refreshButton();
};

init();
