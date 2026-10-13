# SnapTry 网关部署说明

## 要求

- 一台 Linux 服务器（实测 Ubuntu），Go 1.23+
- 一个试穿 API key：[Fitroom](https://platform.fitroom.app)（$0.035/张）或 [fal.ai](https://fal.ai)（$0.075/次）

## 部署

```bash
# 1. 传源码并构建
scp -r gateway/ user@your-server:/opt/snaptry-gateway-src/
ssh user@your-server 'cd /opt/snaptry-gateway-src && go vet ./... && go build -o /opt/snaptry-gateway/snaptry-gateway .'

# 2. 写环境变量（600 权限）
cat > /opt/snaptry-gateway/.env <<'EOF'
PORT=18080
PROVIDER=fitroom
GATEWAY_API_KEY=换成你自己的强随机值
FITROOM_API_KEY=换成你申请的 key
# PROVIDER=falai 时改用 FALAI_API_KEY
EOF
chmod 600 /opt/snaptry-gateway/.env

# 3. systemd 服务（示例 snaptry-gateway.service）
# WorkingDirectory=/opt/snaptry-gateway, EnvironmentFile=/opt/snaptry-gateway/.env,
# ExecStart=/opt/snaptry-gateway/snaptry-gateway, Restart=always
sudo systemctl enable --now snaptry-gateway

# 4. 验证
curl http://127.0.0.1:18080/health
# {"ok":"true","provider":"fitroom","version":"0.1.0"}
```

记得在云厂商安全组放行所选端口的入站 TCP。

## 插件对接

Chrome 加载 `extension/` 后，在弹窗"网关设置"里填 `http://your-server-ip:18080` 和 `GATEWAY_API_KEY`。

## 环境变量一览

| 变量 | 说明 |
|---|---|
| `PORT` | 监听端口，默认 18080 |
| `PROVIDER` | `fitroom`（默认）或 `falai` |
| `GATEWAY_API_KEY` | 插件调用网关的鉴权密钥，自定 |
| `FITROOM_API_KEY` | Fitroom key |
| `FALAI_API_KEY` | fal.ai key（PROVIDER=falai 时用） |
