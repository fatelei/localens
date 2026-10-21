'use strict';
const translations = {
 'zh-CN': {
  windowTitle:'拾物搜索',searchingContent:'正在查找相关内容…',searching:'正在搜索…',search:'搜索本地文件…',fileTypes:'文件类型',searchResults:'搜索结果',drag:'拖动搜索窗口',settings:'设置',back:'返回搜索',title:'拾物设置',quit:'退出',
  all:'全部',document:'文档',image:'图片',video:'视频',audio:'音频',other:'其他',file:'文件',content:'✧ 内容',
  editor:'代码编辑器',systemDefault:'系统默认应用',editorHint:'代码文件使用所选编辑器打开，内容命中时跳到对应行。只列出已安装的受支持编辑器。',line:'第 {line} 行',language:'界面语言',folders:'索引文件夹',addFolder:'＋ 添加文件夹',addFolderLink:'添加索引文件夹 ↗',foldersHint:'默认索引用户目录与已挂载的本地磁盘。系统、缓存和应用包会跳过；移除索引不会删除原始文件。',
  imageOCR:'图片文字识别（OCR）',ocrHint:'开启后在本机识别图片中的文字，支持按文字搜索。关闭会暂停识别，已识别文字仍可搜索。',ocrPaused:'图片文字识别已关闭',contentIndex:'内容索引',contentIndexing:'正在构建全文索引 · 剩余 {count} 个文件',contentIndexReady:'全文索引已就绪',contentIndexError:'内容索引暂时失败，搜索使用兼容模式',ocrIndexing:'正在识别图片文字 · 剩余 {count} 张',ocrReady:'图片文字识别已完成 · 离线运行',ocrPlatform:'仅 macOS 支持',ocrUnsupportedHint:'Windows 和 Linux 暂不支持图片文字识别，图片仍可通过文件名和语义搜索查找。',ocrUnavailable:'当前平台不支持 OCR，目前仅支持 macOS',ocrFailed:'{count} 张图片未能识别',retryOCR:'重新识别图片文字',relatedShow:'相关内容',relatedHide:'收起相关内容',relatedHeading:'相关内容 · 模型扩展',relatedEmpty:'没有更多相关内容',semantic:'语义搜索',enabled:'开启',modelHint:'应用自带 EmbeddingGemma 2，无需安装 Python 或联网下载。用一句话找文档和图片，例如“海边日落的照片”。',
  modelOff:'模型已暂停',modelLoading:'正在加载内置模型…',modelReady:'内置模型已就绪 · 离线运行',modelError:'模型加载失败',loadModel:'加载模型',loading:'加载中…',loaded:'已就绪',stopModel:'暂停模型',retryVectors:'补充内容索引',
  rescan:'更新索引',pauseScan:'暂停扫描',shortcutEdit:'修改',shortcutSave:'保存',shortcutReset:'恢复默认',shortcutCapture:'点击修改后按下组合键',shortcutRecording:'请按下新的快捷键，Esc 取消',shortcutHint:'支持修饰键 + 字母、数字、空格或 F1–F12。保存后立即生效；默认 Ctrl + Q。',shortcutInvalid:'请使用 Ctrl、Alt 或 Command/Windows 加字母、数字、空格或 F1–F12。',shortcutSaved:'快捷键已保存',shortcut:'随时唤起搜索',previewOpen:'打开 ↗',reveal:'在文件夹中显示',
  noRoots:'还没有添加文件夹。',emptyScope:'添加一个文件夹，开始搜索你的本地资料。',indexingEmpty:'正在建立索引，结果会陆续出现。',noResults:'没有找到匹配文件，试试更短的关键词。',
  results:'{count} 个结果',files:'{count} 个文件',vectors:'{count} 个内容向量',scanning:'正在索引 · 已处理 {count} 个文件',embedding:'正在理解内容 · 已生成 {count} 个向量',updated:' · 更新于 {time}',
  filename:'文件名',lexical:'文件名 / 正文',hybrid:'✧ 语义 + 文件名',remove:'移除 {path} 的索引',desktopOnly:'请在拾物桌面应用中使用。',
  keys:'Tab 分类 · ↑ ↓ 选择 · Enter 打开 · ⌘ Y 预览 · Esc 收起',modelNotBundled:'此安装包未包含完整模型，请使用完整安装包。'
 },
 en: {
  windowTitle:'Shiwa Search',searchingContent:'Looking for relevant content…',searching:'Searching…',search:'Search local files…',fileTypes:'File types',searchResults:'Search results',drag:'Drag to move the search window',settings:'Settings',back:'Back to search',title:'Shiwa Settings',quit:'Quit',
  all:'All',document:'Documents',image:'Images',video:'Videos',audio:'Audio',other:'Other',file:'File',content:'✧ Content',
  editor:'Code editor',systemDefault:'System default app',editorHint:'Open code files in the selected editor, at the matching line for content hits. Only installed supported editors are listed.',line:'Line {line}',language:'Interface language',folders:'Indexed folders',addFolder:'+ Add folder',addFolderLink:'Add a folder ↗',foldersHint:'Your home folder and mounted local disks are indexed by default. System folders, caches and app bundles are skipped. Removing an index keeps your files.',
  imageOCR:'Image text recognition (OCR)',ocrHint:'Enable to recognize text in local images for keyword search. Turning it off pauses recognition; existing recognized text remains searchable.',ocrPaused:'Image text recognition is off',contentIndex:'Content index',contentIndexing:'Building full-text index · {count} files remaining',contentIndexReady:'Full-text index ready',contentIndexError:'Content indexing failed temporarily; fallback search remains available',ocrIndexing:'Recognizing image text · {count} images remaining',ocrReady:'Image text recognition complete · Offline',ocrPlatform:'macOS only',ocrUnsupportedHint:'Image text recognition is not supported on Windows or Linux yet. Filename and semantic image search remain available.',ocrUnavailable:'OCR is not supported on this platform. Currently available on macOS only.',ocrFailed:'{count} images could not be recognized',retryOCR:'Recognize image text again',relatedShow:'Related content',relatedHide:'Hide related content',relatedHeading:'Related content · Model suggestions',relatedEmpty:'No additional related content',semantic:'Semantic search',enabled:'On',modelHint:'EmbeddingGemma 2 is included. No Python setup or model download needed. Describe documents or images, such as “photos of a sunset at the beach”.',
  modelOff:'Model paused',modelLoading:'Loading the built-in model…',modelReady:'Built-in model ready · Offline',modelError:'Model failed to load',loadModel:'Load model',loading:'Loading…',loaded:'Ready',stopModel:'Pause model',retryVectors:'Index remaining content',
  rescan:'Update index',pauseScan:'Pause scan',shortcutEdit:'Change',shortcutSave:'Save',shortcutReset:'Reset',shortcutCapture:'Click Change, then press a shortcut',shortcutRecording:'Press a new shortcut. Esc cancels.',shortcutHint:'Use modifiers + a letter, digit, Space or F1–F12. Applies immediately after saving. Default: Ctrl + Q.',shortcutInvalid:'Use Ctrl, Alt or Command/Windows with a letter, digit, Space or F1–F12.',shortcutSaved:'Shortcut saved',shortcut:'Open search anywhere',previewOpen:'Open ↗',reveal:'Show in folder',
  noRoots:'No folders added yet.',emptyScope:'Add a folder to start searching your local files.',indexingEmpty:'Building the index. Results will appear as files are processed.',noResults:'No matching files. Try a shorter search.',
  results:'{count} results',files:'{count} files',vectors:'{count} content vectors',scanning:'Indexing · {count} files processed',embedding:'Understanding content · {count} vectors created',updated:' · Updated at {time}',
  filename:'Filename',lexical:'Filename / text',hybrid:'✧ Semantic + filename',remove:'Remove the index for {path}',desktopOnly:'Please use the Shiwa desktop app.',
  keys:'Tab Type · ↑ ↓ Select · Enter Open · ⌘ Y Preview · Esc Dismiss',modelNotBundled:'This package does not include the full model. Please use the complete app package.'
 }
};
let uiLanguage='zh-CN';
function t(key,values={}){let text=translations[uiLanguage]?.[key]??translations['zh-CN'][key]??key;if(/Win|Linux/i.test(window.navigator?.platform||''))text=text.replaceAll('⌘','Ctrl');for(const [name,value] of Object.entries(values))text=text.replaceAll('{'+name+'}',String(value));return text;}
function applyLanguage(language){
 uiLanguage=language==='en'?'en':'zh-CN';
 document.documentElement.lang=uiLanguage;document.title=t('windowTitle');
 document.querySelectorAll('[data-i18n]').forEach(e=>e.textContent=t(e.dataset.i18n));
 document.querySelectorAll('[data-i18n-placeholder]').forEach(e=>e.setAttribute('placeholder',t(e.dataset.i18nPlaceholder)));
 document.querySelectorAll('[data-i18n-aria]').forEach(e=>e.setAttribute('aria-label',t(e.dataset.i18nAria)));
 document.querySelectorAll('[data-i18n-title]').forEach(e=>e.setAttribute('title',t(e.dataset.i18nTitle)));
 $('language').value=uiLanguage;
}
