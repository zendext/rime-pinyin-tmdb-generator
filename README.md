# rime-pinyin-tmdb-generator

`rime-pinyin-tmdb-generator` 是一个本地生成工具，用于从 TMDb
元数据生成 Rime 词典。它面向 Rime 拼音类方案，包括依赖 Rime 拼写运算的双拼方案。

本仓库不分发任何来自 TMDb 的词典数据。公开发布内容只包含源码、示例配置和服务模板。

## 安装

从源码构建：

```sh
go build -o rime-pinyin-tmdb-generator ./cmd/rime-pinyin-tmdb-generator
```

## 配置

默认配置文件路径是：

```text
~/.config/rime-pinyin-tmdb-generator/config.toml
```

这个默认路径遵循 Linux/XDG 习惯。macOS、Windows 或不想使用默认目录时，可以把配置文件放在任意位置，并在运行时用 `--config` 指定。

可以从示例复制一份：

```sh
mkdir -p ~/.config/rime-pinyin-tmdb-generator
cp examples/config.toml ~/.config/rime-pinyin-tmdb-generator/config.toml
```

然后填写 TMDb API Key：

```toml
[tmdb]
api_key = "你的 TMDb API Key"
base_url = "https://api.themoviedb.org/3"
# 可选：["zh-CN", "zh-TW", "zh-HK"]
languages = ["zh-CN"]

[output]
dict_path = "~/.local/share/rime-data/tmdb.dict.yaml"
lock_path = "~/.local/state/rime-pinyin-tmdb-generator/update.lock"

[bootstrap]
mode = "popular"
request_interval = "200ms"

[bootstrap.popular]
trending_week_pages = 5
popular_pages = 10
top_rated_pages = 10

[rime]
redeploy_command = ""
```

SQLite store 默认按模式分开，通常不需要配置：

- popular 模式：`~/.local/state/rime-pinyin-tmdb-generator/series-popular.sqlite`
- full 模式：`~/.local/state/rime-pinyin-tmdb-generator/series-full.sqlite`

这样可以同时保留两种模式的进度和增量时间线。需要放到其他位置时，可以在配置里增加：

```toml
[store]
path = "/path/to/series.sqlite"
```

`bootstrap.mode = "popular"` 是默认模式，会抓取常见榜单并写入 SQLite store。各榜单的默认页数是：

- `/trending/tv/week`：默认最多 5 页
- `/tv/popular`：默认最多 10 页
- `/tv/top_rated`：默认最多 10 页

这些榜单会按 TMDb ID 去重，再逐个请求 `/tv/{id}/translations`。要构建完整 TV series 词库，可以改成：

```toml
[bootstrap]
mode = "full"
request_interval = "200ms"
```

全量模式会下载 TMDb Daily ID Export，按 ID 逐个请求 `/tv/{id}/translations`。`200ms` 等于 5 rps；一次请求会返回所有配置语言的翻译，所以不会因为配置多个语言而成倍增加请求数。运行状态和进度都保存在 SQLite store 里，遇到中断或 429 后下次运行会从 cursor 继续。

popular 和 full 模式都可以用 `status` 查看本地状态：

```sh
rime-pinyin-tmdb-generator status
```

popular 模式下可以查看最近一次抓取/生成时间、词条数量和 store 中的剧集数量。full 模式下还会显示全量初始化的 export date、cursor 和 completed 状态。当输出里出现下面这一行时，表示当前配置已经可以开启定时任务：

```text
timer_ready=true
```

## 生成词典

```sh
rime-pinyin-tmdb-generator generate \
  --output ~/.local/share/rime-data/tmdb.dict.yaml
```

生成器会按 `dict_path` 所在目录写出词典：

- popular 模式：`tmdb_popular_hans.dict.yaml` / `tmdb_popular_hant.dict.yaml`
- full 模式：`tmdb_full_hans.dict.yaml` / `tmdb_full_hant.dict.yaml`

`dict_path` 只用于决定输出目录；文件名会按当前 `bootstrap.mode` 自动派生。`hans` 为简体，来自 `zh-CN`；`hant` 为繁体，来自 `zh-TW` / `zh-HK`。

popular 模式的简体用户在主 Rime 词典中引入：

```yaml
import_tables:
  - tmdb_popular_hans
```

popular 模式的繁体用户引入：

```yaml
import_tables:
  - tmdb_popular_hant
```

full 模式则对应引入 `tmdb_full_hans` 或 `tmdb_full_hant`。

工具会在 SQLite store 中保存运行状态，并用其中的时间戳进行后续增量更新。只有词典文件成功写入后，状态时间戳才会更新。

## 其他平台

程序本身是 Go CLI，不依赖 Linux 专有 API；Linux 之外的平台建议显式指定配置、输出和 SQLite store 路径。

macOS 示例：

```sh
rime-pinyin-tmdb-generator generate \
  --config "$HOME/Library/Application Support/rime-pinyin-tmdb-generator/config.toml" \
  --output "$HOME/Library/Rime/tmdb.dict.yaml" \
  --store "$HOME/Library/Application Support/rime-pinyin-tmdb-generator/series.sqlite"
```

Windows PowerShell 示例：

```powershell
rime-pinyin-tmdb-generator.exe generate `
  --config "$env:APPDATA\rime-pinyin-tmdb-generator\config.toml" `
  --output "$env:APPDATA\Rime\tmdb.dict.yaml" `
  --store "$env:LOCALAPPDATA\rime-pinyin-tmdb-generator\series.sqlite"
```

无论在哪个平台，实际输出都会在同目录下按当前模式生成，例如 popular 模式为 `tmdb_popular_hans.dict.yaml` 和 `tmdb_popular_hant.dict.yaml`，full 模式为 `tmdb_full_hans.dict.yaml` 和 `tmdb_full_hant.dict.yaml`。

## 拼音修正

生成器会先使用 Go 拼音转换作为自动猜测。专有名词、多音字或自动转换不准确的条目，可以在本地修正文件中维护：

```text
~/.config/rime-pinyin-tmdb-generator/overrides.yaml
```

示例：

```yaml
entries:
  "长安剧场": "chang an ju chang"
  "虚构剧集":
    pinyin: "xu gou ju ji"
```

用户修正会优先生效，覆盖自动拼音。

## 定时更新

`systemd/` 目录包含 user service 和每天运行一次的 timer 模板。复制到 `~/.config/systemd/user/` 后启用 timer：

```sh
systemctl --user enable --now rime-pinyin-tmdb-generator-update.timer
```

如果使用 `bootstrap.mode = "full"`，先手动运行 `generate` 到 `status` 显示 `timer_ready=true`，再启用 timer。

如果使用 systemd 运行，建议直接在 `config.toml` 中配置 `api_key`，或通过 systemd user override 提供 `TMDB_API_KEY`。

## TMDb 使用说明

TMDb API 可以免费用于非商业用途，但需要遵守 TMDb API Terms，包括标注 TMDb 来源、不得商业使用、不得长期缓存或再分发派生数据。这个工具只在用户本地生成词典；请不要把真实生成的 `tmdb_popular_hans.dict.yaml`、`tmdb_popular_hant.dict.yaml`、`tmdb_full_hans.dict.yaml` 或 `tmdb_full_hant.dict.yaml` 作为公开发布文件。
