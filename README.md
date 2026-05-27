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
dir = "~/.local/share/fcitx5/rime"
lock_path = "~/.local/state/rime-pinyin-tmdb-generator/update.lock"

[bootstrap]
mode = "full"
request_interval = "50ms"
min_popularity = 10
movie_min_popularity = 15

[bootstrap.popular]
trending_week_pages = 5
popular_pages = 10
top_rated_pages = 10

[rime]
redeploy_command = ""
```

`output.dir` 是词典输出目录。未配置时，Linux 默认会优先使用已存在的 fcitx5-rime 用户目录：

- 如果 `$XDG_DATA_HOME/fcitx5/rime` 存在：`$XDG_DATA_HOME/fcitx5/rime`
- 否则：`$XDG_DATA_HOME/rime-data`
- 未设置 `$XDG_DATA_HOME` 时按 `~/.local/share` 计算

SQLite store 默认按模式分开，并在 full 模式下按媒体类型分开，通常不需要配置：

- popular 模式 TV：`~/.local/state/rime-pinyin-tmdb-generator/series-popular.sqlite`
- full 模式 TV：`~/.local/state/rime-pinyin-tmdb-generator/series-full.sqlite`
- full 模式电影：`~/.local/state/rime-pinyin-tmdb-generator/movies-full.sqlite`

这样可以同时保留两种模式的进度和生成状态。需要放到其他位置时，可以在配置里增加：

```toml
[store]
path = "/path/to/series.sqlite"
movie_path = "/path/to/movies.sqlite"
```

## 模式

`bootstrap.mode` 决定数据来源、SQLite store 和生成的词典名称。

### full 模式

`bootstrap.mode = "full"` 是默认模式。它会读取 TMDb Daily ID Export，分别生成 TV 和电影词库：

- TV：读取 `tv_series_ids_*`，写入 `series-full.sqlite`，生成 `tmdb_full_hans.dict.yaml` / `tmdb_full_hant.dict.yaml`
- 电影：读取 `movie_ids_*`，写入 `movies-full.sqlite`，生成 `tmdb_movie_hans.dict.yaml` / `tmdb_movie_hant.dict.yaml`
- 默认会尝试今天、昨天和前天的 export；配置 `export_date = "05_26_2026"` 时只使用指定日期

full 模式会先在本地过滤 Daily Export，再请求 translations：

- TV 只保留非成人、有效 ID 且 `popularity >= min_popularity` 的条目，默认 `min_popularity = 10`
- 电影只保留非成人、有效 ID、非 video movie 且 `popularity >= movie_min_popularity` 的条目，默认 `movie_min_popularity = 15`
- 通过过滤后，TV 请求 `/tv/{id}/translations`，电影请求 `/movie/{id}/translations`
- 一次 translations 请求会返回所有配置语言的翻译，所以配置多个语言不会让请求数成倍增加

`request_interval = "50ms"` 表示 translations 请求之间至少间隔 50ms；`50ms` 等于 20 rps。

首次 full bootstrap 遇到中断或 429 后，下次运行会从 SQLite store 里保存的 cursor 继续。首次 bootstrap 完成后，后续运行会重新下载最新可用 Daily Export，并和本地 store 做 diff：新增且达标的 ID 才会请求 translations；已存在的 ID 只更新 popularity；低于阈值、成人、无效、movie video 或从 export 消失的条目会从对应 store 删除。

流行度阈值只在 full 扫描 Daily Export 时生效。断点续传保存的是 export 文件的 cursor，不会保存当时的 `min_popularity` 或 `movie_min_popularity`，所以不要在同一个未完成的 full bootstrap 中途修改这些值：调低阈值不会自动回头补抓之前已跳过的条目。首次 bootstrap 完成后会通过 Daily Export 本地 diff 应用当前阈值，低于阈值或消失的条目会从 SQLite store 删除。

### popular 模式

popular 模式只生成 TV 词库，不生成电影词库。可以这样配置：

```toml
[bootstrap]
mode = "popular"
request_interval = "50ms"

[bootstrap.popular]
trending_week_pages = 5
popular_pages = 10
top_rated_pages = 10
```

popular 模式会抓取常见榜单，按 TMDb ID 去重，再逐个请求 `/tv/{id}/translations` 并写入 `series-popular.sqlite`。各榜单的默认页数是：

- `/trending/tv/week`：默认最多 5 页
- `/tv/popular`：默认最多 10 页
- `/tv/top_rated`：默认最多 10 页

popular 模式生成 `tmdb_popular_hans.dict.yaml` / `tmdb_popular_hant.dict.yaml`。`min_popularity` 和 `movie_min_popularity` 只影响 full 模式，不影响 popular 模式。

popular 和 full 模式都可以用 `status` 查看本地状态：

```sh
rime-pinyin-tmdb-generator status
```

`status` 读取当前模式的 TV store：popular 模式查看 `series-popular.sqlite`，full 模式查看 `series-full.sqlite`。输出里会包含最近一次抓取/生成时间、词条数量和 store 中的剧集数量。full 模式还会显示 TV export 的 date、cursor 和 completed 状态。

首次使用 full 模式时，建议先手动运行 `generate`，确认命令成功返回，再查看 `status`。当输出里出现下面这一行时，表示当前 TV bootstrap 已完成，可以开启定时任务：

```text
timer_ready=true
```

## 生成词典

```sh
rime-pinyin-tmdb-generator generate \
  --output-dir ~/.local/share/fcitx5/rime
```

生成器会按 `dir` 和 `languages` 写出词典：

- `zh-CN`：popular 模式为 `tmdb_popular_hans.dict.yaml`，full 模式为 `tmdb_full_hans.dict.yaml`
- `zh-TW` / `zh-HK`：popular 模式为 `tmdb_popular_hant.dict.yaml`，full 模式为 `tmdb_full_hant.dict.yaml`
- full 模式电影会额外生成 `tmdb_movie_hans.dict.yaml`，配置 `zh-TW` / `zh-HK` 时额外生成 `tmdb_movie_hant.dict.yaml`

文件名会按当前 `bootstrap.mode` 和 `languages` 自动派生。`hans` 为简体，来自 `zh-CN`；`hant` 为繁体，来自 `zh-TW` / `zh-HK`。没有配置繁体语言时，不会生成 `_hant` 词典。

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

full 模式 TV 对应引入 `tmdb_full_hans` 或 `tmdb_full_hant`。电影词库对应引入 `tmdb_movie_hans` 或 `tmdb_movie_hant`。

如果 `output.dir` 是 Rime 用户目录下的子目录，引入时也要带上相对路径。例如 `dir = "~/.local/share/fcitx5/rime/cn_dicts"` 时：

```yaml
import_tables:
  - cn_dicts/tmdb_full_hans
  - cn_dicts/tmdb_movie_hans
```

工具会在 SQLite store 中保存运行状态。只有词典文件成功写入后，状态时间戳才会更新。

## 其他平台

程序本身是 Go CLI，不依赖 Linux 专有 API；Linux 之外的平台建议显式指定配置、输出和 SQLite store 路径。full 模式有 TV 和电影两个 store，`--store` 只覆盖 TV store；如果要同时指定电影 store，请在配置文件里写 `[store].movie_path`。

macOS 配置示例：

```toml
[output]
dir = "~/Library/Rime"

[store]
path = "~/Library/Application Support/rime-pinyin-tmdb-generator/series-full.sqlite"
movie_path = "~/Library/Application Support/rime-pinyin-tmdb-generator/movies-full.sqlite"
```

Windows 配置示例：

```toml
[output]
dir = "~/AppData/Roaming/Rime"

[store]
path = "~/AppData/Local/rime-pinyin-tmdb-generator/series-full.sqlite"
movie_path = "~/AppData/Local/rime-pinyin-tmdb-generator/movies-full.sqlite"
```

无论在哪个平台，实际输出都会在同目录下按当前模式和语言生成，例如只配置 `languages = ["zh-CN"]` 时，full 模式会生成 `tmdb_full_hans.dict.yaml` 和 `tmdb_movie_hans.dict.yaml`。

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

如果使用 `bootstrap.mode = "full"`，先手动运行 `generate` 并确认命令成功返回，再检查 `status` 显示 `timer_ready=true`，然后启用 timer。

如果使用 systemd 运行，建议直接在 `config.toml` 中配置 `api_key`，或通过 systemd user override 提供 `TMDB_API_KEY`。

## TMDb 使用说明

TMDb API 可以免费用于非商业用途，但需要遵守 TMDb API Terms，包括标注 TMDb 来源、不得商业使用、不得长期缓存或再分发派生数据。这个工具只在用户本地生成词典；请不要把真实生成的 `tmdb_popular_hans.dict.yaml`、`tmdb_popular_hant.dict.yaml`、`tmdb_full_hans.dict.yaml`、`tmdb_full_hant.dict.yaml`、`tmdb_movie_hans.dict.yaml` 或 `tmdb_movie_hant.dict.yaml` 作为公开发布文件。
