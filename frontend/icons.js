'use strict';
// Trusted inline SVGs keep file icons crisp without platform fonts or image assets.
function fileIcon(file){
 const extension=file.name.split('.').pop().toLowerCase();
 const types={pdf:['PDF','pdf'],html:['HTML','web'],htm:['HTML','web'],doc:['DOC','office'],docx:['DOC','office'],xls:['XLS','sheet'],xlsx:['XLS','sheet'],ppt:['PPT','slides'],pptx:['PPT','slides'],md:['MD','text'],txt:['TXT','text'],json:['JSON','code'],js:['JS','code'],ts:['TS','code'],go:['GO','code'],py:['PY','code'],zip:['ZIP','archive'],tar:['TAR','archive'],gz:['GZ','archive']};
 const fallback={image:['IMG','image'],video:['VID','video'],audio:['AUD','audio'],document:['DOC','text'],other:['FILE','other']};
 const [label,type]=(Object.hasOwn(types,extension)?types[extension]:null)||(Object.hasOwn(fallback,file.kind)?fallback[file.kind]:fallback.other);
 const icon=node('span','file-icon file-'+type);icon.setAttribute('aria-hidden','true');
 if(type==='video'){
  icon.innerHTML='<svg viewBox="0 0 36 40" fill="none"><rect x="3.5" y="5" width="29" height="30" rx="4" fill="#8b70ae" stroke="#776093" stroke-width="1.1"/><path d="M10 5v30M26 5v30" stroke="white" opacity=".35"/><path d="M16 14.5 23 20l-7 5.5v-11Z" fill="white"/><g fill="white" opacity=".9"><rect x="5.5" y="8" width="3" height="4" rx=".7"/><rect x="5.5" y="18" width="3" height="4" rx=".7"/><rect x="5.5" y="28" width="3" height="4" rx=".7"/><rect x="27.5" y="8" width="3" height="4" rx=".7"/><rect x="27.5" y="18" width="3" height="4" rx=".7"/><rect x="27.5" y="28" width="3" height="4" rx=".7"/></g></svg>';
  return icon;
 }
 icon.innerHTML='<svg viewBox="0 0 36 40" fill="none"><path d="M8 2.5h14l8 8v25a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2v-31a2 2 0 0 1 2-2Z" fill="white" stroke="currentColor" stroke-width="1.2"/><path d="M22 3v7.5h7.5" stroke="currentColor" stroke-width="1.2"/><path d="M11 15h14M11 19h10" stroke="currentColor" opacity=".35" stroke-linecap="round"/></svg>';
 icon.append(node('span','file-badge',label));return icon;
}
