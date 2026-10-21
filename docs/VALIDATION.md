# 开发验证

环境：2026-10-13，macOS arm64，Go 1.27.2，Wails v2.12.0，Python 3.12.12。

已完成：

- `go test -race ./...`：通过。
- `go vet ./...`：通过。
- JavaScript 语法与 Python 编译检查：通过。
- 原生 Wails production 构建、`.app` 打包与本地签名：通过。
- 真机 UI：默认仅悬浮搜索条；原生目录选择器添加测试目录；“押金”搜索命中文档正文；图片文件名搜索与 `⌘ Y` 预览；Esc 隐藏；二次启动唤起现有实例。
- 代码审查发现的文件名候选截断、Unicode 小写、模型停止状态覆盖、失效进程仍显示 ready、双实例扫描冲突均已修复。

全局快捷键注册成功。CUA 自动化发送的 Option+Space 组合未触发 Carbon 热键；实体键盘唤起仍需人工验证。应用菜单和二次启动唤起已验证。

真实模型：通过。已下载完整权重，使用 SentenceTransformers 6.1.0、Transformers 5.19.0、Torch 2.14.1，在 CPU float32 下关闭联网完成文本和图片推理。6 次输出均为 256 维、有限数值、单位归一化向量。中文查询与相关租赁文档的余弦相似度为 0.839，高于无关数据库文档的 0.575；截图查询与图片的相似度为 0.674，高于森林查询的 0.605。单次文本推理约 0.04–0.19 秒，图片约 1.08 秒；这是少量测试样本，不代表完整资料库的召回质量或所有硬件的速度。详细结果见 `docs/model-smoke.json`。

最终键盘审查另外修复了输入法 composition 期间 Enter / 方向键 / Esc 被截获，以及新查询尚未返回时 Enter 打开旧结果的问题。

真实模型应用级集成测试 `TestRealLocalModel`（race 模式）通过：生成 3 个持久化向量，中文语义搜索将租赁合同排在首位，图片描述检索命中截图。JavaScript 键盘回归 3 项通过：IME 确认不打开文件、新查询不打开旧结果、有效 Enter 正常打开。

窗口透明度修复：关闭覆盖整个窗口的 NSVisualEffectView，并在 macOS 主队列设置 NSWindow 非 opaque、透明背景及刷新阴影。初始搜索条、结果展开、设置页和缩回四种状态已进行原生 UI 检查；Go 测试与 vet 通过。

图标更新：加入macOS菜单栏NSStatusItem放大镜（左键搜索、右键搜索/设置/退出菜单）与AppIcon.icns应用图标。原生编译、Go测试及vet通过；AppKit事件由主队列处理，Go回调不阻塞UI。CUA未提供菜单栏状态项的可操作AX节点，图标左/右键点击尚未通过自动化验证。

桌面搜索修复：原症状为索引目录0、目标记录0。默认桌面索引回归先失败后通过；主动移除目录不自动恢复、保留已有索引范围、iCloud下载但size/mtime不变时补正文与chunks、旧库placeholder列迁移均通过。真实窗口已索引桌面27个文件，搜索the_telegraph返回The_Telegraph_SLI_ebook.pdf（占位标志1，正文未下载）；未触发PDF下载。

鼠标拖动修复：全高放大镜拖动区显式声明Wails drag，SVG pointer-events:none；输入框上下释放拖动边缘，同时保留no-drag与文本选择。原生拖动动作已执行，拖动后搜索仍命中桌面PDF，输入框选中文字telegraph正常；3项键盘回归通过，构建通过。

## 完整内置模型发行包与语言切换（2026-10-13）

简体中文 / English 可在设置切换，保存到 settings.json；界面、搜索结果标签与应用菜单跟随选择。Python 路径输入已移除。语义搜索启用偏好持久化，包含加载取消后立即重新开启的回归测试。

完整包包含独立 CPython 3.12、推理依赖和 EmbeddingGemma 2 权重（revision 914f7f89142e33e77833254d9c9b90c3cef7303b）。工作进程强制使用包内 Python 与本地模型路径，禁用模型网络读取和用户 Python 包。

已将完整应用迁移到 /private/tmp（与源码 .venv / .models 分离），通过实际 Go 集成测试 TestRealLocalModel：3 个文件、3 个归一化内容向量，文本与图片检索通过（22.98 秒）。完整应用及嵌套二进制通过 codesign --verify --deep --strict。Go race 测试与 6 个前端测试通过。

完整模型运行已验证的平台为 macOS arm64。运行时底层诊断详情保留原文；部分后端中文诊断尚未提供英文翻译。

原生窗口验证通过：设置中切换 English 后搜索与设置文字立即更新，退出重启仍为 English；随后恢复简体中文。内置模型自动加载就绪并生成 29 个内容向量，保留已有桌面 27 文件索引。完整压缩包约 1.01 GiB，应用保存在 outputs/Localens.app。

## 文件图标与体积优化

用固定 SVG 文件图标与 PDF / HTML 等类型徽标替代字体符号；未知类型回退为通用文件图标。对象原型属性文件名（constructor / __proto__）回归测试通过，前端共 7 项通过。

模型权重从 1,488,915,288 字节精简为 877,599,816 字节。仅移除未使用的 audio_tower / embed_audio tensors，全部保留 tensor 的 dtype、shape 与 SHA256 均与原模型一致。删除运行时头文件与安装工具；torchgen 为运行时依赖，保留。精简后的独立 Python 环境通过 TestRealLocalModel（12.16 秒）：3 文件、3 个归一化向量，离线文本与图片检索通过。

优化版最终 ZIP 为 879.0 MiB（原包约 1.01 GiB），CRC 完整性验证通过；ZIP 内模型压缩 667.2 MiB、Python 与依赖压缩 195.5 MiB。签名严格校验通过。原生窗口确认 PDF 红色徽标、HTML 蓝色徽标均正常显示，保留原搜索 the。

## 默认用户目录与本地磁盘索引

旧桌面范围自动迁移为用户主目录，事务保留现有 files 与 chunks。新增本地磁盘发现（macOS MNT_LOCAL、Windows fixed/removable、Linux 常见本地文件系统）；30 秒周期检查新盘，SeenVolumes 记录确保用户移除后不会恢复。

回归覆盖首次启动 home / Downloads 文件名检索、旧桌面正文保留、主目录视频检索、新盘自动加入和用户移除不恢复、目录直属视频优先登记，以及不可读子树不影响其他区域的删除清理。Go race 测试与 go vet 通过。

原生运行验证：既有 work/ui-data 的 scope 从 /Users/example/Desktop 合并至 /Users/example，已登记 41,859 文件并持续扫描。搜索 Out 时实际用户文件 Out Of Context Football…mp4 位于第一条，类型为视频，无需手动添加目录。桌面原有索引仍保留。

视频文件图标改为固定 SVG 胶片与播放三角，移除 VID 纸张徽标。前端测试和代码复核通过，更新完整安装包并验证 CRC。

## 搜索降噪

移除路径候选、排序与评分；父目录 outputs 不会导致 Out 命中所有视频。短拉丁关键词只匹配文件名词首，避免 Router / layout / leather 等内嵌子串。短词及音视频筛选不调用语义推理，界面显示“文件名”。中文短描述及自然语言句末标点仍可语义查询。

新增对应检索回归，Go race 与前端测试通过，代码双轴复核通过。0.65 保守语义阈值下，实际离线 TestRealLocalModel 文本与图片检索通过（11.70 秒）。

使用实际 work/ui-data/index.db 校验新短词条件：Out + video 仅 1 条，为 Out Of Context Football…mp4。更新包解压后通过 codesign --verify --deep --strict。此次窗口自动访问持续 timeoutReached，未完成原生界面结果验证；检索回归与实际索引查询已完成。

## Dock 唤起修复

Wails 2.12 AppDelegate 没有 applicationShouldHandleReopen:hasVisibleWindows:。在 AppKit 主队列仅为缺失 selector 添加回调，保留原 delegate 与生命周期；回调恢复主窗口、激活应用，并经非阻塞 Go channel 聚焦输入。构建和原生代码复核通过。

原生验证：Escape 后 noWindowsAvailable，随后通过 macOS reopen 操作恢复 Localens Search 与搜索输入。Dock 系统应用无法被自动化工具读取（timeoutReached），未完成物理 Dock 图标点击验证；相同 reopen 事件路径已验证。

## 命中证据与响应修复

结果直接显示原文片段并安全高亮实际出现的查询词；文件名也高亮，路径独立作为位置，不显示正文命中/语义相关标签。非连续词查询片段定位第一个出现词，语义排名覆盖时保留字面正文证据。

新增 file_names 无正文元数据表，触发器在同一事务同步文件增删与更新，独立只读连接池避免索引写入占用查询连接。默认快速阶段只搜索文件名，内容阶段异步执行；新查询取消过期内容 SQL。输入变化立即清空旧列表，响应校验 query/kind/version；轮询不再作废正在等待的搜索。

Go race、11 个前端测试、正文/图片真实模型检索（12.01 秒）通过。真实 140,184 文件索引：Out Of 53.59 ms、The_Telegraph 30.66 ms，Out 224.95 ms；名称匹配中的 out-of-process 是真实文件名命中，视频筛选仅目标 MP4。

原生展示使用明确新建的非敏感 Operations notes 示例：nightly snapshot 查询返回原文段落及高亮，文件名不含关键词也能直观看到依据；右侧仅文件类型，恢复正常用户索引后退出独立示例环境。

正常用户环境恢复后，Out of 查询文件名与对应词高亮已在原生窗口确认；来源不同的 out-of-process、out_of_bounds 等结果能从名字本身判断依据，路径未高亮或参与匹配。

## 2026-10-14 编辑器与代码行号

- Go race tests passed; frontend 13 tests passed.
- Regression coverage: Unicode semantic anchors, semantic passage priority, safe editor argument construction, preference persistence, and current line navigation after edits.
- Native isolated sample.go: searchFiles displayed three code lines with line 4 and keyword highlighting.
- Enter opened sample.go in installed VS Code; native accessibility status reported Ln 4, Col 1.
- Native settings listed System default app, Visual Studio Code, and Zed on this computer. Other editor integrations use documented command formats and were not verified with installed native apps.

## 2026-10-14 搜索结果闪现修复

- Native reproduction: while True produced 80+ content results, then reverted to empty/pending after background status refresh.
- Cause: a new filename-only response overwrote the complete content response during same-query re-search triggered by index status changes.
- Fix: retain complete content results for the same query/type until the next complete content response; new query/type and disabled semantic search still replace results normally.
- Deterministic frontend regression failed before the fix and passed after it. All 14 frontend tests passed; both code reviews passed.
- Updated native app: while True results stayed visible across subsequent background refresh observation.

## 2026-10-14 完整短语优先

- Added regression with 4002 files: 4001 newer scattered-word hits and one older exact `while True` body hit. It failed before the fix and passed after.
- SQL candidate ordering now prioritizes full body phrases before LIMIT 4000; final scores preserve filename phrase priority, then body phrase, then scattered terms and semantic hits.
- Test verifies the exact phrase file ranks first and its snippet targets the phrase at line 3.
- Go race suite and 14 frontend tests passed. Both code reviews passed.

## 2026-10-14 Ctrl + Q 全局唤起

- macOS / Windows / Linux use ModCtrl + KeyQ; settings fallback and README updated.
- Windows/Linux quit menu changed to Ctrl+Shift+Q to avoid collision; macOS quit remains Command+Q.
- Windows/Linux translated shortcut hints replace Command glyph with Ctrl. All 15 frontend tests passed, Go tests passed, native build passed, review passed.
- No Windows desktop runtime was available for native shortcut verification.

## 2026-10-14 自定义快捷键

- Go race suite, go vet and 17 frontend tests passed; two code reviews passed.
- Fake native registration tests cover invalid/duplicate/reserved chords, replacement persistence, occupied-combination retention, and rollback on preference write failure.
- Native macOS UI recorded Ctrl+Alt+P, registered/saved without error, restored the same value after an actual Quit/relaunch, then successfully reset to Ctrl+Q.
- Physical global key events and a Windows native runtime were not independently exercised; synthesized CUA events do not reliably trigger the macOS Carbon global hotkey callback.

## 2026-10-14 macOS 物理快捷键修复

- Physical Control+Q reproduction on macOS 27.0.1: prior library registered successfully but no keydown callback arrived; user confirmed the window did not appear.
- macOS now installs one Carbon handler on GetEventDispatcherTarget with a Localens signature, registers hotkeys on the same target, and forwards callbacks to nonblocking buffered Go channels. Windows/Linux retain the library backend.
- Native registration uses asynchronous AppKit dispatch and context-aware completion; cancellation cleans late registrations without waiting for a stopped main loop.
- ShowLauncher now unhides the application, restores minimized windows, shows the window, and focuses search.
- Physical Control+Q delivered a callback and showed the search window after the fix; user independently confirmed success. Temporary diagnostics were removed.
- Race suite, vet, frontend 17 tests and native build passed. Native tests cover canceled registration without an AppKit loop and nonblocking/inactive callback routing. Review passed.
- Command+Q remains the macOS Quit shortcut and is reserved from global search assignment.


## 2026-10-14 磁盘清理与索引去重

- Removed generated developer model/runtime copies, package staging resources, Go/uv caches, isolated test indexes, obsolete native prototypes and duplicate development builds. Kept source, full release app, installer and active user index.
- The original index contained 448893 files, 1810989 chunks and 14006 vectors. Migration preserved all counts and verified existing vector bytes by SHA256; SQLite quick_check returned ok.
- Index database shrank from 5415505920 bytes to 3037278208 bytes (43.9%). Temporary recovery snapshot removed after successful verification.
- Chunk copies are replaced by Unicode character ranges only after exact text comparison; unknown fallback chunks remain verbatim. New scans persist ranges immediately. Semantic queries reconstruct only final result passages and guard against replaced chunk IDs with path/vector checks.
- Native app reopened successfully; bundled model became ready and both literal and semantic passages rendered. The next scan removed records for the generated caches that were intentionally deleted.
- Go race suite, vet, frontend 17 tests and code reviews passed. Regression tests cover Unicode/emoji, migration vector preservation, fallback chunks, compacted full body and semantic evidence/line navigation.

### Settings return state (2026-10-14)

- Returning without changing search configuration retains results, selection, scroll position, and preview.
- Interrupted searches resume with query/version guards; existing evidence remains until the content response completes. A final filename response is applied when semantic search is off or the model is unavailable.
- Changing indexed roots or semantic configuration refreshes results. Global hotkey return avoids duplicate searches.
- Frontend regression suite: 21 tests passed. Code review passed.
- Native macOS release verified: filename result survived settings return; content query `多词查询优先` retained README evidence, selected result, and open preview after settings return. Signed app and release ZIP updated; temporary build cache removed.

### Explicit related content (2026-10-14)

- Default full keyword retrieval is independent of model availability. SearchRelated has separate vector recall (160 candidates), deduplicated against up to 80 keyword results; appends up to 80 suggestions in a separate section.
- Query/type changes reset expansion; collapsing rejects late responses; expanded refresh retains old rows until completion.
- Go tests and 24 frontend tests passed, including model-off literal body and keyword-cap/dedup vector regressions. Code review passed.
- Native UI verified: query `多词查询优先` returns 3 literal files by default; clicking Related content expands to 83 rows with a separate model-suggestions heading; collapse restores 3. `go vet ./...` passed. Signed app/ZIP updated, temporary compile caches removed.

### Keyboard type filters (2026-10-14)

- Tab cycles file kinds, Shift+Tab reverses; focuses query and shares click-filter logic. Existing query/kind/version guards remain. Settings, blank input, IME, Ctrl/Alt/Meta+Tab retain default behavior.
- Frontend suite: 26 tests passed; code review passed.
- Installed /Applications/Localens.app updated and signature verified. Native UI: Tab changed all→document→image, Shift+Tab reversed to document→all, query focus retained.

### Loading indicator (2026-10-14)

- Regression reproduced: status polling showed loading for background searches and retriggered full keyword searches for every vector-count increment.
- Background refresh is now silent and preserves existing rows. Foreground search and explicit Related content retain progress indication. Vector-count increments alone no longer trigger search; model, root, file count and scan/work state changes still refresh.
- Spinner uses a transparent SVG arc with no border/background and fixed flex size.
- Frontend suite: 30 tests passed; code review passed.
- Native installed app verified: SVG arc appears during foreground query without white sides; disappears on completion; remains hidden across a subsequent background refresh while results remain visible. HTML span controls SVG visibility. App and ZIP signatures verified.

### Recording artifacts and query latency (2026-10-14)

- Reviewed the user-supplied 5-second recording. Animated vertical bright strips were visible around the spinner; single screenshots had missed the artifact.
- Spinner now animates circle stroke-dashoffset without any transform; panel uses opaque #fafafc while the exterior remains transparent. Search input native appearance is disabled.
- Same trimmed input no longer cancels/restarts an effective query (including trailing spaces).
- Uncased body terms use literal instr; cased terms retain Unicode lower and escaped LIKE. Full keyword query reuses file_names.name_lower. macOS CGO uses native SQLite with per-connection functions; other builds retain the portable driver. Database schema unchanged.
- Read-only measurement on actual index (no private text emitted): filename ~55ms, full query `多词查询优先` ~3.0–3.3s; prior store query ~13.3s. Raw Python comparisons did not represent the app runtime and were superseded by Go measurements.
- Frontend 31 tests passed; native and portable driver tests, vet and release build verified below.
- All Go tests, CGO-disabled catalog fallback, catalog race tests and vet passed. Installed app and archive signature verified. Native UI checked across multiple foreground and related-content loading frames: no vertical bright strips observed. Trailing-space input preserved results with no restart.

### File-type tab cache (2026-10-14)

- Single-query cache for six categories stores completed rows, empty results, keyword rows, completed related expansion, selected path and scroll position. Cache hits do not call Search or show loading.
- Query, scope, completed index revision and semantic changes invalidate cached tabs. Request-captured index keys prevent late old results from entering a newer cache revision.
- 36 frontend tests passed, including empty caches, stale tab responses, query/index invalidation and an index update during an in-flight request. Code review verified below.
- Final review passed. Model loading→ready no longer invalidates keyword caches; model-unavailable restoration hides related suggestions. Native installed app verified: after initial index revision stabilized, All↔Document restored completed rows immediately with no loading; initial changed index revisions correctly required a fresh search. Signature and release ZIP verified.


## Full-text index and macOS OCR — 2026-10-14

Native Go tests, race tests and vet passed; portable catalog tests passed; 37 frontend tests passed. Generated bilingual PNG recognized by Apple Vision and searched through the native interface with highlighted OCR snippets. Dual-index mutation, pending fallback, legacy upgrade, Unicode punctuation, stale OCR and compaction/reopen regressions passed. Real 405,599-file database backfilled in approximately five minutes; zero pending full-text records. Document-body and vector hashes unchanged versus pre-migration backup; SQLite quick_check passed. Database file remained 3,037,450,240 bytes by reusing free pages. See README for measured query latency and broad-query limitations. Updated signed app and release ZIP.


## Optional OCR setting — 2026-10-14

OCR now defaults off, including legacy preferences. Native race tests and vet passed, portable catalog tests passed, and 39 frontend tests passed. Regressions cover cancellation of an in-flight result, opt-in scheduling, retaining existing recognized text when paused, independent preference persistence, and stale status protection. Installed app signature verified. Native settings confirmed OCR off initially, enabling resumes pending recognition, disabling pauses it without changing semantic search. Release and source archives updated.


## OCR platform availability label — 2026-10-14

Unsupported OCR displays a macOS-only badge and explicit Windows/Linux limitation, with disabled toggle and retry. Chinese and English platform-state regressions passed; all 40 frontend tests passed. Supported macOS hides the limitation badge. Native build and reviewer checks passed.


## 拾物 / Shiwa rename — 2026-10-14

Product UI, native menus/tray, window titles, binary and package metadata renamed. Chinese UI uses 拾物; English UI uses Shiwa. Bundle ID, single-instance ID, Localens index directory and LOCALENS_* environment overrides remain unchanged. Existing frontend tests (40) and native Go regression tests passed. Complete release re-signed with model assets retained; UTF-8 ZIP names and contents verified.

Native installed verification: /Applications/拾物.app launches as 拾物; settings heading is 拾物设置, English switch changes the window to Shiwa Search and heading to Shiwa Settings. Original 409,757 indexed files, 32,824 vectors, ready bundled model, Ctrl+Q and disabled OCR were retained. Chinese language restored. Installed signature verified; old /Applications/Localens.app renamed in place.


## Intermittent Ctrl+Q investigation — 2026-10-15

One installed process was running with Ctrl+Q saved and no reported registration error. User physically retried Control+Q from another application and confirmed the search appeared. Original intermittent failure was not reproduced; no hotkey root cause is claimed. An independent rename regression was found: native appearance still selected the old Localens Search title. Appearance now selects the Wails delegate mainWindow on the AppKit queue, covering both localized titles without affecting hotkey registration. Native build and shortcut registration/cancellation/replacement tests passed; reviewer checked window selection.

User clarified that the original shortcut failure occurred during normal use, not after lock or sleep. Sleep/wake recovery is not an established cause.

Updated /Applications/拾物.app signature verified and compact localized window launched successfully. Native screenshot confirms normal capsule layout. This does not establish a fix for the unreplicated intermittent global-hotkey failure.


## Bounded search pipeline — 2026-10-15

Final native race suite, portable catalog suite, vet, and 40 frontend tests passed. Verified Unicode phrases/punctuation, mixed name/body terms, exact phrase priority after 8194 false-positive candidates, four concurrent phrase queries, at-most-four evidence workers, cancellation and stale/deleted evidence. Two-axis review passed after fixing reader-cursor starvation and rechecking evidence against current metadata/body; semantic chunk/path/vector guards remain. No index migration or new postings added.

Sequential old/new comparison on the same index with desktop background scanning/inference stopped: old while True 13.211s first / 8.007s repeated; new 2.686s first / 0.728s repeated. New return nil 0.131s; Chinese phrase 多词查询优先 0.039s. Broad or cold queries can still be slower. Temporary profiler and baseline source removed after validation.

Installed /Applications/拾物.app updated and strict signature verified. Native UI search while True returned 80+ results; highlighted prose, Python excerpts and absolute line numbers remain visible. Release ZIP CRC and executable/seal contents verified. No additional full-text postings or database migration required.

## GitHub Actions and native release matrix — 2026-10-15

Added branch/PR checks, reusable lint/test/native-build workflow and v* tag releases for Windows, Linux and macOS on amd64/arm64 native runners. Commit-pinned official actions and weekly Dependabot updates; publication has the only contents-write permission and requires all six packages plus verified SHA-256 checksums. Linux uses WebKitGTK 4.1 and Xvfb. Archives explicitly exclude the model/runtime; macOS packaging reconstructs only whitelisted application files.

Local validation passed: actionlint 1.7.12, source formatting/syntax, Go vet and full macOS race suite, 40 frontend regressions, four packaging regressions, actual macOS ARM64 build/sign/package and Windows amd64/arm64 production cross-compilation with PE architecture validation. Fixed a Windows database file-URI authority bug and added drive/path-escaping tests. Packaging tests account for Windows lacking POSIX execute bits. Two review passes found no remaining concrete issues after fixes.

No Git repository/remote is configured in this workspace, so no hosted Actions run or GitHub release has been executed. Linux native builds, Windows runtime behavior and macOS Intel runtime behavior still require the configured runners and platform acceptance testing. CI artifacts are app-only; existing installed complete macOS app/archive are unchanged.

## Complete offline releases supersede stripped packages — 2026-10-15

User requirement: include the model and inference environment in the download, avoiding first-run network failures in China. Official targets are now Windows amd64/arm64, Linux amd64/arm64 and macOS arm64; macOS Intel removed. The previous app-only release configuration is superseded. Branch CI builds/tests without uploading incomplete apps. Tag releases prepare pinned standalone CPython, constrained CPU dependencies and the pinned model, then require five full archives, SHA-256 checksums and actual offline inference reports.

Application resource discovery now covers Windows/Linux Resources beside the executable, including Windows Python directly under python/. Bundled processes use UTF-8 and offline flags. Packaging validates application/Python architectures and refuses incomplete assets; reconstructed bundles exclude stale developer resources. Embedded macOS binaries are signed before testing the relocated bundle. Smoke tests deny socket connections, use an empty model cache and verify normalized 256-dimensional query/text/image vectors. Runtime preparation retains a generated platform hash lock and installs with required hashes. Windows/Unix build-only asset trimming retains licenses and text/vision tensor bytes.

Local validation: source lint/actionlint, full native macOS Go vet/race tests and 40 frontend tests passed. Packaging/runtime/optimizer regression suite passed (one real Windows cmd-quoting test is intentionally skipped on macOS). Windows amd64 and arm64 production cross-builds passed. Pinned CPython distributions exist for all five targets. Linux ARM64 CPU dependency resolution passed. Windows ARM64 lacked an official torchvision wheel; CI now builds official pinned torchvision source with native MSVC ARM64 before hashed installation. A synthetic torchvision wheel metadata fixture confirmed that the remaining Windows ARM64 dependencies resolve; that fixture is not a compiled binary, is not bundled and does not validate native torchvision execution.

Actual current macOS ARM64 full packaging passed: approximately 871.2 MiB ZIP, relocated Python/model, valid application signature, forbidden network connections, successful query/text/image vectors and archive integrity/checksum. No GitHub remote is connected, so Windows/Linux native inference and the Windows ARM64 torchvision source build remain unverified until hosted jobs run; failure prevents publication rather than producing a stripped package. Existing complete installed macOS app/archive remain unchanged.

Additional build-host verification: private uv 0.9.10 installation and a copied managed CPython 3.12.12 successfully installed/imported a pinned dependency without modifying host Python. The staged interpreter retains the managed-environment marker; installation explicitly overrides it only for that private copy. Regression coverage also checks private interpreter targeting and includes an actual Windows-only batch invocation with a spaced path.

## Directory ownership refactor — 2026-10-15

Root Go source is now limited to the thin Wails-compatible main.go. Application lifecycle/API code and its tests moved into internal/app, split into lifecycle, search, indexing, files and model operations. Native appearance, menus, tray, shortcuts and local-volume adapters (including Objective-C bridges) moved together into internal/desktop. The desktop package receives callbacks/context without importing the app. Shortcut context setters were exported across the package boundary. Tray installation retains the original synchronous submission before language changes; event handling/cleanup remains tied to the app WaitGroup.

UI assets are embedded by frontend; worker source by engine. The Wails namespace is app.App and frontend calls/tests were updated. Regression tests validate the Go type/JavaScript binding contract and all embedded HTML asset references, exempting only Wails' dynamically served runtime endpoint. Build/setup scripts, legacy macOS packaging and asset optimization moved into scripts, with all active references updated. Validation history now lives under docs.

Passed: formatting/syntax, Go vet and full native macOS race suite, 40 frontend regressions, 14 Python packaging/runtime regressions (Windows-only real cmd test skipped on macOS), shell syntax, macOS production build/signature from an external working directory, Windows amd64/arm64 production cross-builds and PE architecture checks. Two code review passes passed after restoring native tray installation ordering. No installed app replacement or user-data migration was required. Native Linux/Windows runtime acceptance remains assigned to their CI runners.


## English README and configuration reference — 2026-10-15

Replaced the mixed-language README with English-only documentation, checked against current application preferences, scope discovery, catalog/indexing, worker and release scripts. It now covers use, platform verification status, settings/defaults, saved JSON, application/development/test environment variables, fixed implementation limits, supported formats, FTS5/literal phrase ranking, vector/OCR behavior, synchronization, caching, build configuration and complete offline releases. Removed obsolete claims that full-text indexing and image OCR were unimplemented. Local links/anchors, language, fenced examples, JSON fields and documented LOCALENS variables were checked; runtime code was not changed.

## Image result thumbnails (2026-10-21)

Added lazy, proportional thumbnails for image rows with icon fallback. Regression
coverage checks request concurrency, reuse after rerender, stale observer callbacks,
failed image requests, bounded output, invalid/oversized sources, cancellation,
cache invalidation, and rejection of unindexed/changed/deleted images. No persistent
thumbnail storage is created.

Validation: 43 frontend tests passed; native `go vet` and all Go race tests
passed; 14 Python tests passed (one Windows-only test skipped on macOS). The new
catalog image authorization test also passed under the race detector. The updated
macOS application was installed, its full signature verified, and a live
`海边日落` / Images / Related Content search visibly displayed photo thumbnails.
