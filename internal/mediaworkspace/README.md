# 章节媒体模块

该目录包含工作台自有的媒体界面资源和素材交换格式。它不启动独立服务，也不读取模型密钥或直接修改项目正文。

## 接入边界

- `ui/media.js`：通过 `NovelMedia.create()` 接收桌面工作台注入的请求、项目上下文、章节列表、导航、通知和保存文件回调。章节切换和异步结果用目录、章节、上下文版本校验，防止串项目。
- `ui/media.css`：配图列表、章节阅读配图、生成任务与独立媒体配置面板样式。
- `Frontend()`：向桌面进程提供嵌入资源，挂载在 `/modules/media/`，与桌面页面使用相同会话校验。
- `CreateArchive()`：只接收章节快照、图片元数据和 `ReadImage` 回调，不依赖具体项目存储、HTTP 服务或绘图引擎。取消、读图失败或校验不一致时删除临时文件。
- `internal/entry/desktopui/chapter_media_export.go`：负责获取项目快照和租约，将项目数据转换为本模块的数据结构。
- `internal/chaptermedia/`：负责项目内图片存储与安全校验。替换编辑器时可以继续使用这些图片，无需改写正文或迁移已有项目。
- `internal/entry/desktopui/chapter_media_generation.go`：读取指定章节已保存的正文，维护异步生成任务、取消、来源校验及同源内容接口。
- `internal/entry/desktopui/media_generation_provider.go`：Images、Videos、Audio Speech 的 OpenAI 兼容请求。Key 只在后端使用，返回图片 URL 的下载不携带 Key。音视频不会改写章节正文。

语音额外支持 `breeze-tts` 协议，对接官方 Breeze TTS 2 `/v1/audio/speech` multipart 接口。配置使用 `instruction` 声音描述、`cfgScale` 与 `seed`，不使用 OpenAI 音色 ID。后端逐段串行请求并将单声道 s16le PCM 封装为 WAV，沿用章节生成的取消、来源校验、大小限制及播放/下载流程。本次提供 Voice Design，不提供参考音频克隆。声音描述来自语音连接设置，图片/视频画面要求不参与音频生成。

推理服务由用户通过 [`scripts/breeze-tts.sh`](../../scripts/breeze-tts.sh) 独立准备和启动，模块不自动安装 Python 或下载模型；运行环境及许可证见 [Breeze TTS 配音接入指南](../../docs/breeze-tts.md)。

模型配置按 image、video、audio 分为三个独立面板。生成请求必须携带项目目录、章节号、媒体类型和正式正文/草稿来源；正文在后端读取，界面预览或章节规划不能冒充正文。每次提交会记录来源 SHA-256，任务卡按项目与章节过滤，并保留正在播放的 audio/video 节点，避免轮询打断播放。

## 素材包

ZIP 内包含 `chapter.json` 与 `images/<图片 ID>.<扩展名>`。`chapter.json` 的 `format` 为 `ainovel-chapter-media`，`schemaVersion` 为 `1`，保存以下信息：

- `chapter`：章号、标题、卷名、已保存正文、正文 SHA256，以及事件、悬念、场景等规划。
- `images`：文件相对路径、原文件名、媒体类型、尺寸、图注、章首/章尾位置、顺序、创建时间和 SHA256。
- `exportedAt`：导出时间。

素材包不携带本机目录、服务地址或 API Key。图片使用原始文件字节，导出前验证大小和 SHA256；图片总量上限为 256 MiB。导出的临时文件权限为仅当前用户读写，桌面另存为后清理。

未来接入新的画布或媒体生成技术时，在本目录添加适配器，通过上述数据结构和宿主回调交互。核心生成引擎、章节版本及项目锁无需跟随绘图技术更换。
