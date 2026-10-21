'use strict';
// Visible rows share a bounded cache and two background requests.
function createThumbnailLoader(fetchImage, makeImage, Observer=globalThis.IntersectionObserver){
 const cache=new Map(),waiting=new Map();let queue=[],active=0,generation=0,observer;
 function pump(){while(active<2&&queue.length){const task=queue.shift();if(task.generation!==generation)continue;task.started=true;active++;Promise.resolve().then(()=>fetchImage(task.file.path)).then(data=>{if(typeof data==='string'&&data.startsWith('data:image/')){cache.set(task.key,data);while(cache.size>128)cache.delete(cache.keys().next().value);for(const item of waiting.get(task.key)||[])if(item.generation===generation)item.show(data);}}).catch(()=>{}).finally(()=>{waiting.delete(task.key);active--;pump();});}}
 function load(item){if(item.generation!==generation)return;const data=cache.get(item.key);if(data){cache.delete(item.key);cache.set(item.key,data);item.show(data);return;}if(waiting.has(item.key)){waiting.get(item.key).push(item);return;}waiting.set(item.key,[item]);queue.push(item);pump();}
 return {
 reset(root){generation++;queue=[];for(const [key,items] of waiting)if(!items.some(item=>item.started))waiting.delete(key);observer?.disconnect();const revision=generation;observer=Observer?new Observer(entries=>{if(revision!==generation)return;for(const e of entries)if(e.isIntersecting){observer.unobserve(e.target);load(e.target.thumbnailItem);}}, {root,rootMargin:'80px'}):null;},
 add(file,host,index){const item={file,key:JSON.stringify([file.path,file.size,file.modified]),generation,show(data){const img=makeImage();img.alt=file.name||'';img.decoding='async';img.onload=()=>{if(item.generation===generation)host.replaceChildren(img);};img.src=data;}};host.thumbnailItem=item;if(observer)observer.observe(host);else if(index<8)load(item);}
 };
}
