'use strict';
const $=id=>document.getElementById(id);
const symbols={document:'▤',image:'▧',video:'▷',audio:'♫',other:'◇'};
let state={tabCache:new Map(),cacheQuery:null,cacheIndexKey:null,renderedIndexKey:null,resultComplete:false,lastResult:null,relatedKey:null,keywordFiles:[],kind:'all',files:[],selected:0,status:null,searchVersion:0,previewVersion:0,refreshKey:'',settings:false,semantic:true,semanticTimer:null,height:0,composing:false,renderedQuery:null,renderedKind:null,renderedContent:false,lexicalInFlight:0,semanticInFlight:0,activeSearchKey:null,inputPending:false,refreshNeeded:false,statusRefreshing:false,ocrSaving:false,ocrVersion:0,editorUpdating:false,editorVersion:0,shortcutRecording:false,shortcutDraft:'',shortcutSaving:false,shortcutVersion:0,settingsSearchKey:null,resumeSettingsSearch:false};
const api=()=>window.go?.app?.App;
async function call(name,...args){if(!api())throw new Error(t('desktopOnly'));return api()[name](...args);}
function node(tag,cls,text){const e=document.createElement(tag);if(cls)e.className=cls;if(text!==undefined)e.textContent=text;return e;}
const thumbnails=createThumbnailLoader(path=>call('GetThumbnail',path),()=>document.createElement('img'));
let toastTimer;
function toast(message){$('toast').textContent=String(message);$('toast').hidden=false;clearTimeout(toastTimer);toastTimer=setTimeout(()=>$('toast').hidden=true,4500);}
async function fit(){
 $('launcher').classList.toggle('has-preview',!$('preview').hidden);
 const height=state.settings?564:Math.min(600,Math.ceil($('launcher').getBoundingClientRect().height+16));
 if(state.height!==height){state.height=height;try{await call('SetPanelHeight',height);}catch(e){console.warn(e);}}
}
function resetPreview(){state.previewVersion++;$('preview').hidden=true;}
function renderResults(result,query=$('query').value.trim(),content=false,indexKey=state.renderedIndexKey){
 state.renderedIndexKey=indexKey;state.lastResult=result;state.resultComplete=content||(result.contentEligible??result.semanticEligible)===false;
 const oldPath=state.files[state.selected]?.path;state.files=result.files||[];state.selected=Math.max(0,state.files.findIndex(f=>f.path===oldPath));const container=$('results');thumbnails.reset(container);container.replaceChildren();state.renderedQuery=query;state.renderedKind=state.kind;state.renderedContent=content;const expanded=Boolean(query);$('launcher').classList.toggle('expanded',expanded);$('search-panel').hidden=!expanded;$('empty').hidden=state.files.length>0||!expanded;
 const hasRoots=Boolean(state.status?.roots.length);$('empty-message').textContent=!hasRoots?t('emptyScope'):state.status?.scanning?t('indexingEmpty'):t('noResults');$('empty-add').hidden=hasRoots;
 $('results-summary').textContent=t('results',{count:state.files.length+(state.files.length===80?'+':'')});$('search-mode').textContent=result.filenameOnly?t('filename'):t('lexical');updateRelated();
 state.files.forEach((f,i)=>{const row=node('button','result-row'+(i===state.selected?' selected':''));row.setAttribute('role','option');row.setAttribute('aria-selected',String(i===state.selected));row.dataset.index=String(i);if(f.related&&(!i||!state.files[i-1].related))container.append(node('div','related-heading',t('relatedHeading')));if(f.kind==='image'){const thumb=node('span','result-thumbnail');thumb.append(fileIcon(f));row.append(thumb);thumbnails.add(f,thumb,i);}else row.append(fileIcon(f));const text=fileEvidence(f,query);row.append(text,node('span','result-tag',t(f.kind==='other'?'file':f.kind)),node('span','result-arrow','↵'));row.onclick=()=>select(i);row.ondblclick=()=>openSelected();container.append(row);});
 if(oldPath!==state.files[state.selected]?.path)resetPreview();fit();
 if(result.warning)toast(result.warning);
}
function select(index){state.selected=Math.max(0,Math.min(state.files.length-1,index));document.querySelectorAll('.result-row').forEach((r,i)=>{const selected=i===state.selected;r.classList.toggle('selected',selected);r.setAttribute('aria-selected',String(selected));});document.querySelectorAll('.result-row')[state.selected]?.scrollIntoView({block:'nearest'});if(!$('preview').hidden)showPreview();}
function clearOldResults(){
 const query=$('query').value.trim();
 if(state.renderedQuery===query&&state.renderedKind===state.kind)return;
 state.files=[];state.selected=0;state.renderedQuery=null;state.renderedKind=null;state.relatedKey=null;state.keywordFiles=[];$('related-toggle').hidden=true;thumbnails.reset($('results'));$('results').replaceChildren();resetPreview();
 $('launcher').classList.toggle('expanded',Boolean(query));$('search-panel').hidden=!query;$('empty').hidden=true;
 $('results-summary').textContent=query?t('searching'):'';$('search-mode').textContent='';$('search-spinner').hidden=!query;fit();
}
async function search(preserve=false,background=false){
 if(state.composing||state.settings)return;
 const query=$('query').value.trim(),kind=state.kind,key=JSON.stringify([query,kind]);
 if(state.lexicalInFlight===state.searchVersion&&state.lexicalInFlight&&state.activeSearchKey===key)return;
 const retainContent=state.renderedQuery===query&&state.renderedKind===kind&&((preserve&&state.files.length>0)||(state.renderedContent&&state.semantic));
 const indexKey=state.cacheIndexKey,version=++state.searchVersion;state.inputPending=false;state.activeSearchKey=key;
 clearTimeout(state.semanticTimer);state.semanticTimer=null;
 const current=()=>version===state.searchVersion&&query===$('query').value.trim()&&kind===state.kind&&!state.settings&&!state.composing;
 if(state.cacheQuery!==query){state.tabCache.clear();state.cacheQuery=query;}
 if(!query){renderResults({files:[]},'');$('search-spinner').hidden=true;return;}
 clearOldResults();$('search-spinner').hidden=background;state.lexicalInFlight=version;
 try{
  const result=await call('Search',query,kind,false);
  if(!current()||(result.query!==undefined&&result.query!==query))return;
  state.relatedEligible=Boolean(result.semanticEligible);
  const contentFollows=result.contentEligible??result.semanticEligible;
  if(!retainContent||!contentFollows)renderResults(result,query,false,indexKey);
  if(contentFollows){
   if(!state.files.length)$('empty-message').textContent=t('searchingContent');
   state.semanticTimer=setTimeout(async()=>{
    state.semanticTimer=null;if(!current())return;state.semanticInFlight=version;
    try{const keywords=await call('SearchKeywords',query,kind);if(current()&&(keywords.query===undefined||keywords.query===query)){state.keywordFiles=keywords.files||[];if(state.relatedKey===key&&state.semantic&&state.status?.model==='ready'){await loadRelated(version,query,kind,background,indexKey);}else{state.relatedKey=null;renderResults(keywords,query,true,indexKey);}}}
    catch(e){if(current())toast(e);}
    finally{if(state.semanticInFlight===version)state.semanticInFlight=0;if(current())$('search-spinner').hidden=true;}
   },300);
  }else $('search-spinner').hidden=true;
 }catch(e){if(current()){toast(e);$('search-spinner').hidden=true;}}
 finally{if(state.lexicalInFlight===version)state.lexicalInFlight=0;}
}
function updateRelated(){
 const query=$('query').value.trim(),key=JSON.stringify([query,state.kind]);
 const button=$('related-toggle');button.hidden=!query||!state.semantic||state.status?.model!=='ready'||!state.renderedContent||!SemanticAllowed();
 button.textContent=t(state.relatedKey===key?'relatedHide':'relatedShow');button.setAttribute('aria-expanded',String(state.relatedKey===key));
}
function SemanticAllowed(){return state.kind!=='video'&&state.kind!=='audio'&&state.relatedEligible;}
async function loadRelated(version,query,kind,background=false,indexKey=state.renderedIndexKey){
 const key=JSON.stringify([query,kind]);if(!state.semantic||state.status?.model!=='ready'){state.relatedKey=null;updateRelated();return;}state.semanticInFlight=version;$('search-spinner').hidden=background;
 try{const result=await call('SearchRelated',query,kind);
  if(version!==state.searchVersion||state.settings||state.relatedKey!==key||query!==$('query').value.trim()||kind!==state.kind||(result.query!==undefined&&result.query!==query))return;
  const paths=new Set(state.keywordFiles.map(f=>f.path));const extras=(result.files||[]).filter(f=>!paths.has(f.path)&&f.match==='语义').slice(0,80).map(f=>({...f,related:true}));
  renderResults({...result,files:[...state.keywordFiles,...extras]},query,true,indexKey);
  if(!extras.length&&!background)toast(t('relatedEmpty'));
 }catch(e){if(version===state.searchVersion&&!state.settings&&state.relatedKey===key){state.relatedKey=null;renderResults({files:state.keywordFiles},query,true,indexKey);toast(e);}}
 finally{if(state.semanticInFlight===version)state.semanticInFlight=0;if(version===state.searchVersion)$('search-spinner').hidden=true;}
}
$('related-toggle').onclick=()=>{
 const query=$('query').value.trim(),kind=state.kind,key=JSON.stringify([query,kind]);
 if(state.relatedKey===key){state.relatedKey=null;state.searchVersion++;renderResults({files:state.keywordFiles},query,true);$('search-spinner').hidden=true;}
 else{state.relatedKey=key;updateRelated();loadRelated(state.searchVersion,query,kind);}
};
async function showPreview(){const f=state.files[state.selected];if(!f)return;const version=++state.previewVersion;$('preview').hidden=false;$('preview-name').textContent=f.name;$('preview-path').textContent=f.path;$('preview-visual').replaceChildren(node('span','',symbols[f.kind]||'◇'));fit();try{const p=await call('GetPreview',f.path);if(version!==state.previewVersion)return;if(p.image){const img=node('img');img.onload=fit;img.src=p.image;img.alt=f.name;$('preview-visual').replaceChildren(img);}else if(p.text){$('preview-visual').replaceChildren(node('pre','',p.text));}fit();}catch(e){if(version===state.previewVersion)toast(e);}}
async function openSelected(reveal=false){if(state.composing||state.renderedQuery!==$('query').value.trim()||state.renderedKind!==state.kind)return;const f=state.files[state.selected];if(!f)return;try{if(reveal)await call('RevealFile',f.path);else if(state.status?.editor)await call('OpenCode',f.path,(f.match==='正文'||f.match==='语义')?state.renderedQuery:'',f.snippet||'');else await call('OpenFile',f.path);if(!reveal)await hide();}catch(e){toast(e);}}
async function hide(){state.searchVersion++;clearTimeout(state.semanticTimer);try{await call('HideLauncher');}catch(e){toast(e);}}
function settingsSearchKey(){return JSON.stringify([state.semantic,(state.status?.roots||[]).map(root=>root.path).sort()]);}
function showSettings(){
 if(state.settings)return;
 state.settingsSearchKey=settingsSearchKey();state.resumeSettingsSearch=Boolean(state.inputPending||state.lexicalInFlight||state.semanticInFlight||state.semanticTimer);
 state.searchVersion++;clearTimeout(debounce);clearTimeout(state.semanticTimer);state.semanticTimer=null;state.lexicalInFlight=0;state.semanticInFlight=0;$('search-spinner').hidden=true;
 state.settings=true;$('launcher').hidden=true;$('settings').hidden=false;fit();refreshStatus();refreshEditors();
}
async function refreshEditors(){try{const editors=await call('GetEditors');const select=$('editor');select.replaceChildren();const fallback=node('option','',t('systemDefault'));fallback.value='';select.append(fallback);editors.forEach(editor=>{const option=node('option','',editor.name);option.value=editor.id;select.append(option);});const saved=state.status?.editor||'';if(saved&&!editors.some(editor=>editor.id===saved)){const unavailable=node('option','',saved);unavailable.value=saved;unavailable.disabled=true;select.append(unavailable);}select.value=saved;}catch(e){toast(e);}}
$('editor').onchange=async()=>{state.editorUpdating=true;state.editorVersion++;const chosen=$('editor').value;try{await call('SetEditor',chosen);if(state.status)state.status.editor=chosen;await refreshStatus();}catch(e){$('editor').value=state.status?.editor||'';toast(e);}finally{state.editorUpdating=false;}};
function closeSettings(){
 const changed=state.settingsSearchKey!==settingsSearchKey();
 const resume=state.resumeSettingsSearch||state.renderedQuery!==$('query').value.trim()||state.renderedKind!==state.kind;
 state.resumeSettingsSearch=false;state.shortcutRecording=false;state.shortcutDraft='';$('shortcut-save').disabled=true;
 state.settings=false;$('settings').hidden=true;$('launcher').hidden=false;fit();$('query').focus();
 if(changed){state.tabCache.clear();state.renderedContent=false;state.refreshNeeded=false;search();}else if(resume)search(true);
}
async function addFolder(){try{const p=await call('ChooseDirectory');if(p){showSettings();await refreshStatus();}}catch(e){toast(e);}}
async function refreshStatus(){
 if(state.statusRefreshing)return;state.statusRefreshing=true;const editorVersion=state.editorVersion,shortcutVersion=state.shortcutVersion,ocrVersion=state.ocrVersion;
 try{const s=await call('GetStatus');if(state.ocrSaving||ocrVersion!==state.ocrVersion)s.ocrEnabled=state.status?.ocrEnabled;if(state.shortcutSaving||shortcutVersion!==state.shortcutVersion){s.shortcut=state.status?.shortcut;s.shortcutError=state.status?.shortcutError;}if(state.editorUpdating||editorVersion!==state.editorVersion)s.editor=state.status?.editor||'';state.status=s;if(state.settings&&!state.editorUpdating)$('editor').value=s.editor||'';if(s.language&&s.language!==uiLanguage)applyLanguage(s.language);state.semantic=s.semanticEnabled!==false;updateRelated();$('semantic').checked=state.semantic;const roots=$('roots');roots.replaceChildren();if(!s.roots.length)roots.append(node('p','hint',t('noRoots')));
 s.roots.forEach(r=>{const row=node('div','root-row');row.append(node('span','root-icon','▱'));const detail=node('div','root-detail');detail.append(node('strong','',r.path.split(/[\\/]/).filter(Boolean).pop()||r.path),node('small','',r.path));row.append(detail,node('span','root-count',t('files',{count:r.count})));const remove=node('button','root-remove','×');remove.setAttribute('aria-label',t('remove',{path:r.path}));remove.onclick=async()=>{try{await call('RemoveDirectory',r.path);await refreshStatus();}catch(e){toast(e);}};row.append(remove);roots.append(row);});
 $('index-status').textContent=s.scanning?t('scanning',{count:s.progress.processed}):s.embedding?t('embedding',{count:s.embedded}):t('files',{count:s.stats.files})+(s.lastScan?t('updated',{time:s.lastScan}):'');
 $('content-status').textContent=s.stats.contentError?t('contentIndexError'):s.stats.textPending?t('contentIndexing',{count:s.stats.textPending}):t('contentIndexReady');
 $('ocr-platform').hidden=Boolean(s.stats.ocrAvailable);$('ocr-hint').textContent=t(s.stats.ocrAvailable?'ocrHint':'ocrUnsupportedHint');$('ocr').checked=s.ocrEnabled===true;$('ocr').disabled=!s.stats.ocrAvailable||state.ocrSaving;
 $('ocr-status').textContent=!s.stats.ocrAvailable?t('ocrUnavailable'):!s.ocrEnabled?t('ocrPaused'):s.stats.ocrPending?t('ocrIndexing',{count:s.stats.ocrPending}):t('ocrReady');if(s.ocrEnabled&&s.stats.ocrFailed)$('ocr-status').textContent+=' · '+t('ocrFailed',{count:s.stats.ocrFailed});$('retry-ocr').disabled=!s.stats.ocrAvailable||!s.ocrEnabled||Boolean(s.stats.ocrPending);
 $('index-error').hidden=!s.error&&!s.progress.skipped;$('index-error').textContent=s.error||s.progress.warnings?.join('\n')||'';$('cancel-scan').hidden=!s.scanning;$('rescan').disabled=s.scanning||s.embedding;
 const modelLabels={off:'modelOff',loading:'modelLoading',ready:'modelReady',error:'modelError'};$('model-label').textContent=t(modelLabels[s.model]||'modelOff');$('model-dot').className='status-dot '+s.model;$('vector-count').textContent=t('vectors',{count:s.stats.vectors});
 $('start-model').disabled=s.model==='loading'||s.model==='ready';$('start-model').textContent=s.model==='loading'?t('loading'):s.model==='ready'?t('loaded'):t('loadModel');$('stop-model').disabled=s.model==='off';$('retry-vectors').disabled=s.model!=='ready'||s.embedding||s.scanning;
 $('model-hint').textContent=s.bundledModel?t('modelHint'):t('modelNotBundled');$('model-error').hidden=!s.modelError;$('model-error').textContent=s.modelError||'';$('shortcut').textContent=s.shortcut||'Ctrl + Q';if(!state.shortcutRecording&&!state.shortcutDraft&&!state.shortcutSaving)$('shortcut-input').value=s.shortcut||'Ctrl + Q';$('shortcut-error').hidden=!s.shortcutError;$('shortcut-error').textContent=s.shortcutError||'';
 const cacheIndexKey=JSON.stringify([(s.roots||[]).map(r=>r.path).sort(),s.stats.contentRevision??s.lastScan??'',s.semanticEnabled]);if(state.cacheIndexKey!==null&&state.cacheIndexKey!==cacheIndexKey)state.tabCache.clear();state.cacheIndexKey=cacheIndexKey;
 const key=JSON.stringify([s.stats.files,s.roots,s.model,s.scanning,s.embedding,s.stats.contentRevision]);if(key!==state.refreshKey){state.refreshKey=key;state.refreshNeeded=true;}
 if(state.refreshNeeded&&!state.settings&&!state.inputPending&&!state.composing&&!state.lexicalInFlight&&!state.semanticInFlight&&!state.semanticTimer){state.refreshNeeded=false;await search(true,true);}
 }catch(e){$('index-error').hidden=false;$('index-error').textContent=String(e);}
 finally{state.statusRefreshing=false;}
}
let debounce;$('query').addEventListener('input',()=>{const query=$('query').value.trim(),key=JSON.stringify([query,state.kind]);if(!state.composing&&!state.inputPending&&state.activeSearchKey===key&&(state.lexicalInFlight||state.semanticInFlight||state.semanticTimer||(state.renderedQuery===query&&state.renderedKind===state.kind)))return;if(state.cacheQuery!==query){state.tabCache.clear();state.cacheQuery=query;}state.searchVersion++;state.inputPending=true;clearTimeout(state.semanticTimer);state.semanticTimer=null;clearTimeout(debounce);clearOldResults();if(!state.composing)debounce=setTimeout(search,120);});
$('query').addEventListener('compositionstart',()=>{state.composing=true;state.searchVersion++;state.inputPending=true;clearTimeout(debounce);clearTimeout(state.semanticTimer);state.semanticTimer=null;clearOldResults();});
$('query').addEventListener('compositionend',()=>{state.composing=false;state.inputPending=true;clearOldResults();clearTimeout(debounce);debounce=setTimeout(search,120);});
function rememberTab(){
 const query=$('query').value.trim();if(state.renderedIndexKey!==state.cacheIndexKey||!state.resultComplete||state.renderedQuery!==query||state.renderedKind!==state.kind)return;
 if(state.cacheQuery!==query){state.tabCache.clear();state.cacheQuery=query;}
 const key=JSON.stringify([query,state.kind]);state.tabCache.set(state.kind,{result:{...state.lastResult,files:[...state.files]},content:state.renderedContent,keywords:[...state.keywordFiles],eligible:state.relatedEligible,related:state.relatedKey===key&&state.lastResult?.semantic===true,indexKey:state.renderedIndexKey,selectedPath:state.files[state.selected]?.path,scroll:$('results').scrollTop||0});
}
function changeKind(kind){
 if(kind===state.kind){$('query').focus();return;}
 rememberTab();state.kind=kind;document.querySelectorAll('[data-kind]').forEach(b=>{const selected=b.dataset.kind===kind;b.classList.toggle('selected',selected);b.setAttribute('aria-pressed',String(selected));});$('query').focus();
 const query=$('query').value.trim(),cached=state.cacheQuery===query?state.tabCache.get(kind):null;
 if(!cached){search();return;}
 state.searchVersion++;clearTimeout(debounce);clearTimeout(state.semanticTimer);state.semanticTimer=null;state.lexicalInFlight=0;state.semanticInFlight=0;state.inputPending=false;state.activeSearchKey=JSON.stringify([query,kind]);
 resetPreview();state.keywordFiles=[...cached.keywords];state.relatedEligible=cached.eligible;state.relatedKey=cached.related&&state.semantic&&state.status?.model==='ready'?state.activeSearchKey:null;
 const result=cached.related&&!state.relatedKey?{...cached.result,semantic:false,files:cached.keywords}:cached.result;
 state.files=[...result.files];state.selected=Math.max(0,state.files.findIndex(f=>f.path===cached.selectedPath));
 renderResults(result,query,cached.content,cached.indexKey);$('results').scrollTop=cached.scroll;$('search-spinner').hidden=true;
}

document.querySelectorAll('[data-kind]').forEach(b=>b.onclick=()=>changeKind(b.dataset.kind));
$('settings-open').onclick=showSettings;$('settings-close').onclick=closeSettings;$('add-root').onclick=addFolder;$('empty-add').onclick=addFolder;
$('rescan').onclick=async()=>{try{await call('Rescan');await refreshStatus();}catch(e){toast(e);}};$('cancel-scan').onclick=async()=>{try{await call('StopScan');await refreshStatus();}catch(e){toast(e);}};
$('semantic').onchange=async()=>{try{await call('SetSemanticEnabled',$('semantic').checked);await refreshStatus();search();}catch(e){toast(e);await refreshStatus();}};
$('language').onchange=async()=>{try{await call('SetLanguage',$('language').value);applyLanguage($('language').value);state.refreshKey='';await refreshStatus();search();}catch(e){toast(e);}};
$('start-model').onclick=async()=>{try{await call('SetSemanticEnabled',true);await refreshStatus();}catch(e){toast(e);}};$('stop-model').onclick=async()=>{try{await call('SetSemanticEnabled',false);await refreshStatus();}catch(e){toast(e);}};$('retry-vectors').onclick=async()=>{try{await call('RebuildVectors');await refreshStatus();}catch(e){toast(e);}};
$('preview-open').onclick=()=>openSelected();$('preview-reveal').onclick=()=>openSelected(true);$('quit').onclick=()=>call('QuitApp');
function recordShortcut(e){
 e.preventDefault();e.stopPropagation?.();
 if(e.key==='Escape'){state.shortcutRecording=false;state.shortcutDraft='';$('shortcut-input').setAttribute('placeholder',t('shortcutCapture'));$('shortcut-input').value=state.status?.shortcut||'Ctrl + Q';$('shortcut-save').disabled=true;return;}
 if(['Control','Alt','Shift','Meta'].includes(e.key))return;
 const code=e.code||'';let key=code==='Space'?'Space':/^Key[A-Z]$/.test(code)?code.slice(3):/^Digit[0-9]$/.test(code)?code.slice(5):/^F([1-9]|1[0-2])$/.test(code)?code:'';
 if(!key||(!e.ctrlKey&&!e.altKey&&!e.metaKey)){toast(t('shortcutInvalid'));return;}
 const mods=[];if(e.ctrlKey)mods.push('Ctrl');if(e.altKey)mods.push('Alt');if(e.shiftKey)mods.push('Shift');if(e.metaKey)mods.push('Meta');
 state.shortcutDraft=[...mods,key].join('+');state.shortcutRecording=false;$('shortcut-input').setAttribute('placeholder',t('shortcutCapture'));$('shortcut-input').value=state.shortcutDraft.replace('Meta',/Mac/i.test(window.navigator?.platform||'')?'Cmd':'Win').replaceAll('+',' + ');$('shortcut-save').disabled=false;
}
$('shortcut-edit').onclick=()=>{state.shortcutRecording=true;state.shortcutDraft='';$('shortcut-input').value='';$('shortcut-input').setAttribute('placeholder',t('shortcutRecording'));$('shortcut-save').disabled=true;$('shortcut-input').focus();};
async function saveShortcut(value){
 if(state.shortcutSaving)return;state.shortcutSaving=true;state.shortcutVersion++;state.shortcutRecording=false;
 ['shortcut-edit','shortcut-save','shortcut-reset'].forEach(id=>$(id).disabled=true);
 try{await call('SetShortcut',value);state.shortcutDraft='';const fresh=await call('GetStatus');if(state.status){state.status.shortcut=fresh.shortcut;state.status.shortcutError=fresh.shortcutError;}$('shortcut').textContent=fresh.shortcut;$('shortcut-input').value=fresh.shortcut;$('shortcut-error').hidden=!fresh.shortcutError;$('shortcut-error').textContent=fresh.shortcutError||'';toast(t('shortcutSaved'));
 }catch(e){toast(e);}
 finally{state.shortcutSaving=false;$('shortcut-edit').disabled=false;$('shortcut-reset').disabled=false;$('shortcut-save').disabled=!state.shortcutDraft;}
}
$('shortcut-save').onclick=()=>saveShortcut(state.shortcutDraft);$('shortcut-reset').onclick=()=>saveShortcut('Ctrl+Q');
document.addEventListener('keydown',e=>{
 if(state.shortcutRecording){recordShortcut(e);return;}

 if(e.isComposing||e.keyCode===229||state.composing)return;
 if((e.metaKey||e.ctrlKey)&&e.key===','){e.preventDefault();showSettings();return;}
 if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='k'){e.preventDefault();if(state.settings)closeSettings();$('query').focus();$('query').select();return;}
 if(e.key==='Escape'){e.preventDefault();if(state.settings)closeSettings();else if(!$('preview').hidden){resetPreview();fit();}else hide();return;}
 if(state.settings)return;
 if((e.key==='ArrowDown'||e.key==='ArrowUp')&&state.files.length){e.preventDefault();select(state.selected+(e.key==='ArrowDown'?1:-1));}
 if(e.key==='Enter'&&state.files.length){e.preventDefault();openSelected(e.metaKey||e.ctrlKey);}
 if(((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='y'||e.code==='Space'&&document.activeElement!==$('query'))&&state.files.length){e.preventDefault();if($('preview').hidden)showPreview();else{resetPreview();fit();}}
 if(e.key==='Tab'&&!e.ctrlKey&&!e.metaKey&&!e.altKey&&$('query').value.trim()){e.preventDefault();const kinds=['all','document','image','video','audio','other'];const index=kinds.indexOf(state.kind);changeKind(kinds[(index+(e.shiftKey?-1:1)+kinds.length)%kinds.length]);}
});
window.localensFocus=()=>{if(state.shortcutRecording)return;const fromSettings=state.settings;if(fromSettings)closeSettings();$('query').focus();$('query').select();if(!fromSettings)search();};
applyLanguage('zh-CN');refreshStatus();setInterval(refreshStatus,1500);fit();

$('retry-ocr').onclick=async()=>{try{await call('RetryImageOCR');await refreshStatus();}catch(e){toast(e);}};

$('ocr').onchange=async()=>{if(state.ocrSaving)return;const enabled=$('ocr').checked;state.ocrVersion++;state.ocrSaving=true;if(state.status)state.status.ocrEnabled=enabled;$('ocr').disabled=true;try{await call('SetOCREnabled',enabled);}catch(e){toast(e);}finally{state.ocrSaving=false;await refreshStatus();$('ocr').disabled=!state.status?.stats.ocrAvailable;}};
