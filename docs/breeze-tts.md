# Breeze TTS 章节配音

本项目内置了 [Breeze TTS 2](https://github.com/breezeblue-ai/breeze-tts) 配音适配器，可从声音描述生成中文或英文旁白。它使用官方推理服务，支持章节分段、取消、播放和 WAV 下载；本次接入为 **Voice Design（声音设计）**，没有参考音频上传或声音克隆功能。

应用安装包不包含 Python、PyTorch、第三方推理源码或模型权重。需要在本机或另一台 Linux GPU 机器上准备并运行 Breeze 服务，再在应用里选择它。Windows、Android 也可以连接这台服务器。

## 运行条件

- 推理服务器：Linux、Python 3.10 或更新版本、Git、Python `venv` 与 `pip`。
- 可供 PyTorch 使用的 NVIDIA CUDA GPU。上游推荐普通 eager 推理使用至少 12 GB 显存；`--fast-all` 推荐 24 GB。官方 API 即使使用 eager 路径也要求 CUDA，不能在纯 CPU 机器上运行。
- 安装 Python 依赖和首次下载权重时需要网络及足够磁盘空间。源码很小，但 PyTorch 依赖和模型权重需要数 GB 空间。
- Ubuntu/Debian 若提示缺少 `venv` 或音频系统库，可安装对应 Python 版本的 `python3-venv`、`libsndfile1`，并确认 NVIDIA 驱动正常。本项目封装 WAV 不需要额外调用 FFmpeg。

上游源码采用 Apache-2.0；模型权重、衍生模型及自部署生成的输出受 [BreezeBlue Research and Non-Commercial License](https://huggingface.co/BreezeBlue/breeze-tts-2/blob/main/LICENSE) 单独约束，仅允许研究及非商业用途；商业使用需要 BreezeBlue 的单独书面授权。应用的 AGPL 许可证不会替代模型许可证。

## 首次安装和启动

在本项目目录执行：

```bash
# 只安装固定版本源码与 Python 依赖，不下载模型权重。
bash scripts/breeze-tts.sh install

# 检查依赖、PyTorch CUDA 和 API 导入；不下载或加载模型。
bash scripts/breeze-tts.sh check

# 前台启动；默认模型首次启动时从 Hugging Face 下载权重。
bash scripts/breeze-tts.sh start
```

默认安装目录为 `$XDG_DATA_HOME/ainovel/breeze-tts`，未设置 `XDG_DATA_HOME` 时为 `$HOME/.local/share/ainovel/breeze-tts`。`source/` 保存上游源码，`venv/` 保存独立 Python 环境。脚本固定上游提交 `daa6cb2efe5471f09369229f322d20bb956cb3c3`，不依赖随时变化的 `main`；重复执行 `install` 会检查并复用完整安装。已有源码如版本、来源或工作区有变化，脚本会停止并保留它，使用新的 `BREEZE_TTS_HOME` 可另建安装。

`start` 默认运行 `BreezeBlue/breeze-tts-2`，监听 `127.0.0.1:7860`，保持终端开启；按 `Ctrl+C` 停止。模型通过 Hugging Face 标准缓存复用，不放入本项目目录。准备好的本地 checkpoint 必须包含上游要求的 `audio_tokenizer/` 等完整文件：

```bash
BREEZE_TTS_MODEL=/absolute/path/to/breeze-tts-2 bash scripts/breeze-tts.sh start
```

等终端显示服务启动完成后，在另一个终端检查：

```bash
curl --fail http://127.0.0.1:7860/health
```

正常返回包含 `"status":"ok"` 与 `"sample_rate":24000`。首次模型下载、加载和快速模式预热可能需要较长时间。仅执行 `check` 成功不代表模型已经加载或完成真实语音推理。

## 在应用中选择配音

1. 打开 **语音模型设置**，新增连接，在 **配音方式** 中选择 **Breeze TTS 2（本地 / 自部署）**。
2. Base URL 填写 `http://127.0.0.1:7860/v1`。在其他设备上使用时，将 `127.0.0.1` 换成 GPU 服务器地址。
3. 设置声音描述，例如“自然清晰的中文旁白，语速适中，语气温和，适合有声小说”。描述语言尽量与正文一致。Breeze 由服务器启动时决定加载哪个模型，不支持通过客户端模型 ID 切换模型，也不使用 OpenAI 的 `alloy` 等音色 ID。
4. CFG 强度默认 `4`，允许大于 `0` 且不超过 `10`；随机种子界面默认 `42`。固定种子会在整章各段复用；留空时后端为本章选一个随机种子，再在各段中复用。相同设置有助音色连贯，但不能保证每次或跨段声音完全一致。
5. 保存并设为默认语音连接，打开需要配音的章节，在 **本章朗读** 中选择已保存的正式正文或草稿，点击 **生成本章朗读**。声音描述用于调整语气、情绪和节奏；图片或视频的画面补充要求不参与配音。

官方本地服务不要求 API Key；如自己部署了鉴权代理，可填对应 Key。保存设置不会启动推理服务、下载权重或发起配音请求。

长章会按约 240 字、优先句子边界自动分段，按顺序请求官方接口，再将单声道 PCM 合并为一个 WAV 文件。生成结果沿用原有章节任务卡，可播放、下载、取消或删除。WAV 是未压缩音频，比 MP3 更大；单任务总运行时限为 20 分钟，音频文件上限为 100 MiB，特别长的章节可拆成多个章节生成。

## 从 Windows、手机或其他电脑连接

在 GPU 服务器上安装并启动服务：

```bash
BREEZE_TTS_HOST=0.0.0.0 bash scripts/breeze-tts.sh start
```

客户端 Base URL 使用 `http://服务器局域网IP:7860/v1`。手机的 `127.0.0.1` 指向手机本身，因此跨设备时必须改为服务器地址。

官方 API 没有自带身份验证；在可信局域网中限制可访问设备，需要跨公网时使用 VPN 或提供 TLS 和鉴权的反向代理。默认 `127.0.0.1` 仅供同机访问。

## 环境变量

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `BREEZE_TTS_HOME` | 用户数据目录下的 `ainovel/breeze-tts` | 独立安装目录，必须为本项目目录之外的绝对路径 |
| `BREEZE_TTS_PYTHON` | `python3` | 安装时用于创建 venv 的 Python 可执行文件 |
| `BREEZE_TTS_MODEL` | `BreezeBlue/breeze-tts-2` | 本地 checkpoint 路径或 Hugging Face repo ID |
| `BREEZE_TTS_REVISION` | 未指定 | HF 模型的分支、标签或 commit；本地路径忽略此项 |
| `BREEZE_TTS_HOST` | `127.0.0.1` | API 监听地址 |
| `BREEZE_TTS_PORT` | `7860` | API 端口 |
| `BREEZE_TTS_FAST` | `0` | `1` 启用上游 `--fast-all`，需要更多显存和预热时间 |
| `HF_HOME` | Hugging Face 默认缓存 | 模型下载缓存目录 |
| `CUDA_VISIBLE_DEVICES` | 当前可见 GPU | 由 PyTorch 使用的 GPU，可限制到指定卡 |

自定义 `BREEZE_TTS_HOME` 时，安装、检查与启动都应使用相同值。安装失败后可以修复环境并重跑 `install`；如果首次创建 venv 因缺少 `python3-venv` 而中断，补装系统组件后重跑即可，脚本会为已有虚拟环境补齐 pip。如果进程被强制杀死留下 `.install-lock`，确认安装进程已停止后再手动删除这个空锁目录。

## 常见问题

- **连接失败**：确认服务终端仍在运行，先请求 `/health`；核对地址、端口、防火墙，以及客户端是否误用自己的 `127.0.0.1`。
- **CUDA 不可用**：运行 `bash scripts/breeze-tts.sh check`；检查 NVIDIA 驱动、GPU 可见性与 PyTorch 安装。应用本身不能把 CPU 变成 CUDA 设备。
- **显存不足**：使用默认 `BREEZE_TTS_FAST=0`，关闭其他占显存任务；本接入逐段发送，不启用上游显存占用更高的批量接口。
- **HTTP 409**：上游服务只允许一个推理请求同时运行。当前应用会串行使用同一个 Breeze 服务；其他应用占用时，等待它完成后重试。
- **404 或返回非音频内容**：确认选择的是 Breeze TTS 协议、Base URL 指向官方 `/v1`，而不是 OpenAI 兼容 JSON 服务或网页首页。
- **听感不稳定或漏读**：用一两句话描述声音，保持适中语速、固定种子和一致的正文语言。过长描述或极慢语速仍可能触及上游序列/音频长度上限；分段不能保证模型绝不漏读，生成后应试听。本次声音设计不提供参考音频克隆，也不保证长篇角色音色锁定。

## 接口与验证范围

适配器向 `POST /v1/audio/speech` 发送 multipart 字段 `text`、`instruction`、`cfg_scale`、`seed`，依据官方返回的音频头校验 PCM 并封装 WAV。上游单次生成有音频 token 和总序列上限，因此应用不把整章直接提交为一条请求。

脚本的离线 smoke 可使用 `python3 scripts/test_breeze_tts.py` 执行；它以 stub 替代 Git 下载、Python 安装和推理调用，不需要 GPU，也不下载依赖或权重。真正的 GPU 启动与音质验证需要上述运行环境和模型。
