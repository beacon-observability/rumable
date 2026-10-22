# Linux Web + Nginx + Rumable 本地演示

访问地址：[http://127.0.0.1:18874/](http://127.0.0.1:18874/)。只绑定本机回环地址。

该目录提供两个 Linux 容器：Node Web 服务返回业务 HTML 和 API，Nginx 反向代理后由正式 Rumable 0.1.5 安装器注入 SDK。业务 HTML 不写 SDK 初始化代码。上报地址指向同一演示服务的模拟接收端，使用虚拟 appId/token、100% 会话采样、关闭回放，不向真实观测云应用发送数据。

## 启动与接入

开发机需有 Docker Compose、Node.js（支持原生 ESM、TextEncoder 与 btoa）和 curl。无需在 Linux 容器内编译 Rumable。

在项目根目录执行：

```sh
bash examples/local-demo/start.sh
```

脚本拉取镜像，启动服务，读取 OSS 0.1.5 清单，调用项目实际页面生成器，执行生成的一键命令。CLI 和 SDK 来自 OSS 正式包。首次使用网络下载，后续保留版本化安装与状态；重复执行复用同一接入标识。

浏览器打开首页，点击“获取商品列表”“慢请求”“演示错误”或进入第二个页面。Network 筛选 `/v1/write/rum`，等待 SDK 批量发送。页面右侧从模拟接收端读取实际 POST 数量、时间和字节数；204 只表示模拟端已接收，不表示真实观测云入库。解析不了的载荷仍计请求数，不伪造事件分类。演示错误按钮产生的 Console error 是预期测试行为。

接收记录在 Web 容器内存中，最多显示最近 40 条；重启 Web 容器清零。模拟端不验证应用或 Token，不应用于生产。

生成的 TOML、实际命令、安装日志、status/probe 在项目 `.local-demo/`，已忽略 Git，不会进入 OSS 发布包。

## 检查、停用与重新启用

```sh
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml ps
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml exec -T nginx /opt/rumable/0.1.5/rumable status --state-dir /var/lib/rumable
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml logs --tail 20 web

# 停止新页面注入，保留演示站点及状态：
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml exec -T nginx /opt/rumable/0.1.5/rumable disable --state-dir /var/lib/rumable
# 重新执行 start.sh 会按原配置恢复接入。
```

后端原始 HTML 与 Nginx 响应可以分别读取，确认只有经过 Nginx 的响应被注入。实际接入走 Rumable，测试服务不自行添加 `data-rumable` 标签。

## 停止与移除

```sh
# 停止服务，保留容器和所有状态：
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml stop

# 移除本演示的容器和网络，保留状态卷：
docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml down
```

确认不再需要演示与回滚证据后，才给上述 down 加 `--volumes` 删除本演示的命名卷。不要使用影响其他项目的 Docker 全局清理命令。服务配置为 `unless-stopped`，Docker 可用时持续运行，主动 stop 后不会自行重新启动。
