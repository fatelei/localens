'use strict';
// Construct highlights as text nodes, never interpreting indexed content as HTML.
function highlightText(container,text,query){
 text=String(text||'');
 const terms=[...new Set(query.trim().split(/\s+/).filter(Boolean))].sort((a,b)=>b.length-a.length);
 if(!terms.length){container.append(node('span','',text));return;}
 const escaped=terms.map(term=>term.replace(/[.*+?^${}()|[\]\\]/g,'\\$&'));
 const expression=new RegExp(escaped.join('|'),'giu');let offset=0;
 for(const match of text.matchAll(expression)){
  if(match.index>offset)container.append(node('span','',text.slice(offset,match.index)));
  container.append(node('mark','match-highlight',match[0]));offset=match.index+match[0].length;
 }
 if(offset<text.length)container.append(node('span','',text.slice(offset)));
}
function fileEvidence(file,query){
 const text=node('div','result-text');const name=node('div','result-name');highlightText(name,file.name,query);text.append(name);
 if(file.snippet&&(file.match==='正文'||file.match==='语义')){
  const excerpt=node('div','result-excerpt'+(file.line?' code-excerpt':''));if(file.line){text.append(node('div','code-location',t('line',{line:file.line})));}if(file.snippetLine){file.snippet.split('\n').forEach((line,index)=>{const row=node('div','code-line');row.append(node('span','line-number',String(file.snippetLine+index)));const code=node('span','code-text');highlightText(code,line,query);row.append(code);excerpt.append(row);});}else highlightText(excerpt,file.snippet,query);
  excerpt.setAttribute('title',file.snippet);text.append(excerpt);
 }
 text.append(node('div','result-description',file.path));return text;
}
