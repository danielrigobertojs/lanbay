<!-- SPDX-License-Identifier: Apache-2.0 -->
<script>
  let password='', currentPassword='', csrf='', error='', changing=false, volumes=[], path='', entries=[], page=1, total=0, newName='', hidden=false, system=false, jobs=[], uploads=[], totalUpload=0, uploaded=0, uploadSpeed=0, uploadStarted=0, dragSource=''
  const headers=()=>({'Content-Type':'application/json','X-CSRF-Token':csrf})
  async function login(){const r=await fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({password})});if(!r.ok){error=await r.text();return};const d=await r.json();csrf=d.csrf_token;changing=d.password_change_required;if(!changing){connectEvents();await refresh()}}
  async function changePassword(){const r=await fetch('/api/v1/auth/password',{method:'POST',headers:headers(),body:JSON.stringify({current_password:currentPassword,password})});if(!r.ok){error=await r.text();return};changing=false;currentPassword='';password='';await refresh()}
  async function refresh(){const r=await fetch('/api/v1/volumes');if(!r.ok){error=await r.text();return};volumes=(await r.json()).volumes;if(!path&&volumes.length)open(volumes[0].id);await refreshJobs()}
  async function refreshJobs(){const r=await fetch('/api/v1/jobs');if(r.ok)jobs=(await r.json()).jobs}
  function connectEvents(){const e=new EventSource('/api/v1/events');e.addEventListener('job',event=>{const j=JSON.parse(event.data);jobs=[j,...jobs.filter(x=>x.id!==j.id)]});e.onerror=()=>setTimeout(connectEvents,1500)}
  async function collectDropped(dataTransfer){
    const files=[]
    async function walkEntry(entry,prefix){
      if(entry.isFile){
        const file=await new Promise((resolve,reject)=>entry.file(resolve,reject))
        Object.defineProperty(file,'relativePath',{value:prefix?prefix+'/'+entry.name:entry.name})
        files.push(file)
      }else if(entry.isDirectory){
        const dirPath=prefix?prefix+'/'+entry.name:entry.name
        const reader=entry.createReader()
        const childEntries=await new Promise((resolve,reject)=>reader.readEntries(resolve,reject))
        for(const child of childEntries) await walkEntry(child,dirPath)
      }
    }
    const items=[...dataTransfer.items]
    let used=false
    for(const item of items){
      const entry=item.webkitGetAsEntry?.()
      if(entry){used=true;await walkEntry(entry,'')}
    }
    if(!used){
      for(const f of dataTransfer.files){
        Object.defineProperty(f,'relativePath',{value:f.name})
        files.push(f)
      }
    }
    return files
  }
  async function ensureDir(parent,relativeDir){
    let cur=parent
    for(const part of relativeDir.split('/').filter(Boolean)){
      const r=await fetch('/api/v1/files/mkdir',{method:'POST',headers:headers(),body:JSON.stringify({parent:cur,name:part})})
      if(!r.ok){const text=await r.text();if(!/exist/i.test(text)) throw new Error(text)}
      cur=cur+'\\'+part
    }
    return cur
  }
  async function upload(file,parentOverride){
    const chunk=8*1024*1024
    const relative=file.relativePath||file.name
    const parts=relative.split('/')
    const name=parts.pop()
    const dir=parts.join('/')
    const targetParent=dir?await ensureDir(parentOverride||path,dir):(parentOverride||path)
    const create=await fetch('/api/v1/uploads',{method:'POST',headers:headers(),body:JSON.stringify({parent:targetParent,name,size_bytes:file.size,chunk_size:chunk})})
    if(!create.ok){error=await create.text();return}
    let session=await create.json(),offset=0
    uploads=[...uploads,{name:relative,size:file.size,offset:0,state:'subiendo'}]
    while(offset<file.size){
      const start=offset,end=Math.min(offset+chunk,file.size),body=file.slice(start,end)
      const r=await fetch('/api/v1/uploads/'+session.id,{method:'PATCH',headers:{'X-CSRF-Token':csrf,'Upload-Offset':String(start)},body})
      if(!r.ok){error=await r.text();return}
      offset=Number(r.headers.get('Upload-Offset'))
      uploaded+=offset-start
      uploadSpeed=uploaded/Math.max(1,(Date.now()-uploadStarted)/1000)
      uploads=uploads.map(x=>x.name===relative?{...x,offset,state:r.headers.get('Upload-State')}:x)
    }
    await open(path)
    await refreshJobs()
  }
  async function dropped(event){
    event.preventDefault()
    const source=event.dataTransfer.getData('text/plain')
    const target=event.currentTarget?.dataset?.path||path
    if(source && source!==target){
      await transfer(source,target,event.altKey)
      return
    }
    const files=await collectDropped(event.dataTransfer)
    totalUpload=files.reduce((n,f)=>n+f.size,0)
    uploaded=0
    uploadStarted=Date.now()
    for(const file of files) await upload(file)
  }
  async function transfer(source,target,copy){
    const name=source.split('\\').pop().split('/').pop()
    const endpoint=copy?'/api/v1/files/copy':'/api/v1/files/move'
    const r=await fetch(endpoint,{method:'POST',headers:headers(),body:JSON.stringify({source,parent:target,name})})
    if(!r.ok){error=await r.text();return}
    await open(path)
    await refreshJobs()
  }
  async function open(next,p=1){path=next;page=p;const q=new URLSearchParams({path,page:p,per_page:100,hidden,system});const r=await fetch('/api/v1/files?'+q);if(!r.ok){error=await r.text();return};const d=await r.json();entries=d.entries;total=d.total;page=d.page}
  function crumbs(){const s=path.replaceAll('\\','/'), p=s.split('/').filter(Boolean);return p.map((x,i)=>({x,path:s.match(/^[A-Za-z]:\//)?p.slice(0,i+1).join('/')+'/':'/'+p.slice(0,i+1).join('/')}))}
  async function mkdir(){if(!newName)return;const r=await fetch('/api/v1/files/mkdir',{method:'POST',headers:headers(),body:JSON.stringify({parent:path,name:newName})});if(!r.ok){error=await r.text();return};newName='';open(path)}
  async function rename(e){const name=prompt('Nuevo nombre',e.name);if(!name)return;const r=await fetch('/api/v1/files/rename',{method:'POST',headers:headers(),body:JSON.stringify({path:e.path,name})});if(!r.ok){error=await r.text();return};open(path)}
  async function remove(e){const volume=volumes.find(v=>e.path.toLowerCase().startsWith(v.id.toLowerCase()));const permanent=!volume?.recycle;const scope=permanent?`Borrar permanentemente 1 elemento: ${e.name}?`:`Enviar a la Papelera 1 elemento: ${e.name}?`;if(!confirm(scope))return;const r=await fetch('/api/v1/files',{method:'DELETE',headers:headers(),body:JSON.stringify({path:e.path,permanent})});if(!r.ok){error=await r.text();return};open(path)}
  function dragStart(e,entry){dragSource=entry.path;e.dataTransfer.setData('text/plain',entry.path);e.dataTransfer.effectAllowed='copyMove'}
  function dragOver(e){e.preventDefault();e.dataTransfer.dropEffect=e.altKey?'copy':'move'}
  function dropOnEntry(e,entry){e.preventDefault();e.stopPropagation();const source=e.dataTransfer.getData('text/plain')||dragSource;if(!source||!entry.directory||source===entry.path)return;transfer(source,entry.path,e.altKey);dragSource=''}
  function dropOnCrumb(e,crumb){e.preventDefault();e.stopPropagation();const source=e.dataTransfer.getData('text/plain')||dragSource;if(!source||source===crumb.path)return;transfer(source,crumb.path,e.altKey);dragSource=''}
  function dropOnTable(e){e.preventDefault();const source=e.dataTransfer.getData('text/plain')||dragSource;if(!source){dropped(e);return};transfer(source,path,e.altKey);dragSource=''}
</script>
<svelte:head><title>Lanbay</title></svelte:head>
{#if !csrf}<main class="login"><section><h1>Lanbay</h1><label>Contraseña <input type="password" bind:value={password}/></label><button on:click={login}>Iniciar sesión</button>{#if error}<p class="error">{error}</p>{/if}</section></main>
{:else if changing}<main class="login"><section><h1>Cambia la contraseña</h1><label>Actual <input type="password" bind:value={currentPassword}/></label><label>Nueva <input type="password" bind:value={password}/></label><button on:click={changePassword}>Guardar</button>{#if error}<p class="error">{error}</p>{/if}</section></main>
{:else}<main><header><h1>Archivos</h1><button on:click={refresh}>Actualizar</button></header>{#if error}<p class="error">{error}</p>{/if}<nav>{#each crumbs() as crumb,i}<button class="link" draggable="true" on:dragover={dragOver} on:drop={(e)=>dropOnCrumb(e,crumb)} on:click={()=>open(crumb.path)}>{crumb.x}</button>{#if i<crumbs().length-1} / {/if}{/each}</nav><section class="tools"><select bind:value={path} on:change={e=>open(e.target.value)}>{#each volumes as v}<option value={v.id}>{v.id} · {v.label||'Volumen local'}</option>{/each}</select><input placeholder="Nueva carpeta" bind:value={newName}/><button on:click={mkdir}>Crear carpeta</button><label><input type="checkbox" bind:checked={hidden} on:change={()=>open(path)}/> ocultos</label><label><input type="checkbox" bind:checked={system} on:change={()=>open(path)}/> sistema</label></section><div class="drop" role="region" aria-label="Subir archivos y carpetas" data-path={path} on:dragover|preventDefault={dragOver} on:drop={dropped}>Arrastra archivos o carpetas aquí para subirlos por chunks reanudables (Alt al soltar mueve/copiar dentro del panel).{#if totalUpload}<progress value={uploaded} max={totalUpload}></progress><small>{Math.round(uploadSpeed/1024/1024)} MB/s · {uploadSpeed?Math.ceil((totalUpload-uploaded)/uploadSpeed):'—'} s restantes</small>{/if}</div><p>{total} elementos · página {page}</p><table><thead><tr><th>Nombre</th><th>Tamaño</th><th>Acciones</th></tr></thead><tbody on:dragover|preventDefault={dragOver} on:drop={dropOnTable}>{#each entries as entry}<tr draggable="true" on:dragstart={(e)=>dragStart(e,entry)} on:dragover={dragOver} on:drop={(e)=>dropOnEntry(e,entry)}><td>{#if entry.directory}<button class="link" on:click={()=>open(entry.path)}>{entry.name}/</button>{:else}{entry.name}{/if}</td><td>{entry.directory?'—':entry.size}</td><td><button on:click={()=>rename(entry)}>Renombrar</button> <button class="danger" on:click={()=>remove(entry)}>Borrar</button></td></tr>{:else}<tr><td colspan="3">Carpeta vacía</td></tr>{/each}</tbody></table><section><h2>Actividad</h2>{#each jobs.filter(j=>j.state==='running'||j.state==='queued') as job}<p>{job.type}: {job.completed_bytes}/{job.total_bytes} · {job.message}</p>{:else}<p>Sin jobs activos.</p>{/each}</section><footer><button disabled={page<=1} on:click={()=>open(path,page-1)}>Anterior</button><button disabled={page*100>=total} on:click={()=>open(path,page+1)}>Siguiente</button></footer></main>{/if}
<style>:global(*){box-sizing:border-box}:global(body){margin:0;background:#0b1020;color:#e9eefb;font:16px system-ui,sans-serif}main{max-width:1000px;margin:auto;padding:2rem}.login{min-height:100vh;display:grid;place-items:center}.login section{padding:2rem;background:#151d33;border-radius:1rem}header,.tools,footer{display:flex;gap:1rem;flex-wrap:wrap;align-items:center}table{width:100%;margin:1rem 0;border-collapse:collapse;background:#151d33}th,td{padding:.8rem;border-bottom:1px solid #283554;text-align:left}input,select,button{padding:.65rem;border-radius:.4rem;border:1px solid #405275;background:#151d33;color:inherit}button{cursor:pointer}.link{border:0;background:none;color:#72d4ff;padding:0}.danger,.error{color:#ff9b9b}.drop{margin:1rem 0;padding:2rem;border:2px dashed #405275;border-radius:.7rem;text-align:center}.drop progress{display:block;width:100%;margin-top:1rem}</style>