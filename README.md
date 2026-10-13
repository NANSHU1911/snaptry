# SnapTry 拍照试穿 MVP

Chrome 插件 + Go 网关的虚拟试穿最小可用原型：在任意服装网站拖一件衣服，拍/传一张上身照，AI 生成试穿效果图（非实时，约 10~60 秒出结果）。

## 架构

```
Chrome 插件 (Manifest V3) ──▶ Go 网关 (自家服务器) ──▶ 试穿 API (Fitroom / fal.ai)
```

- **extension/** — 插件：popup（拍照/上传/结果展示）、content script（服装站拖拽抓衣服图）、background（调网关 + 轮询）
- **gateway/** — Go 网关（纯标准库）：鉴权、限流、Provider 抽象、结果缓存

## 快速开始

1. 申请试穿 API key：[Fitroom](https://platform.fitroom.app)（$0.035/张，credit 不过期）或 [fal.ai](https://fal.ai)（$0.075/次）
2. 部署网关：见 [DEPLOY.md](DEPLOY.md)
3. Chrome 打开 `chrome://extensions` → 开发者模式 → 加载已解压的扩展程序 → 选择 `extension/` 目录
4. 插件弹窗里填网关地址 + 密钥，开始试穿

## 隐私

照片会上传到海外 AI 试穿服务器处理。插件内有中文隐私告知，须勾选同意后才能使用。

## License

MIT
