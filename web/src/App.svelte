<!-- SPDX-License-Identifier: Apache-2.0 -->
<script>
  let password = ''
  let currentPassword = ''
  let csrf = ''
  let error = ''
  let volumes = []
  let system = null
  let alertPercent = 90
  let changing = false
  const format = value => `${(value / 1073741824).toFixed(1)} GB`
  async function login() {
    error = ''
    const response = await fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password }) })
    if (!response.ok) { error = await response.text(); return }
    const data = await response.json(); csrf = data.csrf_token; changing = data.password_change_required; password = ''
    if (!changing) await refresh()
  }
  async function changePassword() {
    const response = await fetch('/api/v1/auth/password', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf }, body: JSON.stringify({ current_password: currentPassword, password }) })
    if (!response.ok) { error = await response.text(); return }; changing = false; password = ''; currentPassword = ''; await refresh()
  }
  async function refresh() {
    const [systemResponse, volumeResponse] = await Promise.all([fetch('/api/v1/system'), fetch('/api/v1/volumes')])
    if (!systemResponse.ok || !volumeResponse.ok) { error = 'No se pudo actualizar el panel.'; return }
    system = await systemResponse.json(); const data = await volumeResponse.json(); volumes = data.volumes; alertPercent = data.alert_percent
  }
</script>

<svelte:head><title>Lanbay</title></svelte:head>
{#if !csrf}
  <main class="login"><section><p class="eyebrow">PANEL LAN</p><h1>Lanbay</h1><p>Administra el equipo desde tu red local.</p><label>Contraseña <input type="password" bind:value={password} on:keydown={(event) => event.key === 'Enter' && login()} /></label>{#if error}<p class="error">{error}</p>{/if}<button on:click={login}>Iniciar sesión</button></section></main>
{:else if changing}
  <main class="login"><section><p class="eyebrow">PRIMER ACCESO</p><h1>Cambia la contraseña</h1><label>Contraseña actual<input type="password" bind:value={currentPassword} /></label><label>Nueva contraseña (12 caracteres mínimo)<input type="password" bind:value={password} /></label>{#if error}<p class="error">{error}</p>{/if}<button on:click={changePassword}>Guardar contraseña</button></section></main>
{:else}
  <main><header><div><p class="eyebrow">ALMACENAMIENTO</p><h1>Buenas, {system?.hostname || 'Lanbay'}</h1><p>Actualización automática cada 30 segundos.</p></div><button class="secondary" on:click={refresh}>Actualizar</button></header><section class="summary"><span>Equipo: {system?.platform || '—'}</span><span>Versión: {system?.version || '—'}</span><span>Certificado: {system?.certificate_fingerprint || '—'}</span></section><section class="grid">{#each volumes as volume}<article class:warning={volume.used_percent >= alertPercent}><div class="card-header"><span class="disk">{volume.id}</span><span>{volume.type}</span></div><h2>{volume.label || 'Volumen local'}</h2><div class="meter"><i style={`width:${volume.used_percent}%`}></i></div><strong>{volume.used_percent}% usado</strong><p>{format(volume.used())} usados de {format(volume.total)}</p><footer><span>{volume.filesystem}</span><span>{format(volume.free)} libres</span></footer>{#if volume.recycle}<small>Papelera disponible</small>{:else}<small class="muted">Borrado permanente requiere confirmación</small>{/if}</article>{:else}<p>No hay volúmenes locales disponibles.</p>{/each}</section></main>
{/if}

<style>
  :global(*){box-sizing:border-box}:global(body){margin:0;background:#0b1020;color:#e9eefb;font:16px/1.5 system-ui,sans-serif}main{max-width:1120px;margin:auto;padding:clamp(1.25rem,4vw,4rem)}header{display:flex;gap:2rem;justify-content:space-between;align-items:center;margin-bottom:2rem}.login{min-height:100vh;display:grid;place-items:center}.login section{width:min(100%,28rem);padding:2rem;background:#151d33;border:1px solid #283554;border-radius:1rem}h1{margin:.1rem 0;font-size:clamp(2rem,6vw,3.7rem);letter-spacing:-.06em}h2{font-size:1.15rem}.eyebrow{color:#72d4ff;font-size:.75rem;font-weight:800;letter-spacing:.16em}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(235px,1fr));gap:1rem}article{padding:1.25rem;border:1px solid #283554;border-radius:1rem;background:#151d33}.card-header,footer,.summary{display:flex;justify-content:space-between;gap:.6rem;color:#aab8d5;font-size:.86rem}.disk{color:#72d4ff;font-weight:800}.meter{height:.55rem;overflow:hidden;background:#26334e;border-radius:2rem;margin:1rem 0 .45rem}.meter i{display:block;height:100%;background:#6ee7b7;border-radius:inherit}.warning .meter i{background:#fbbf24}.warning{border-color:#805e19}small{display:block;margin-top:1rem;color:#72d4ff}.muted{color:#aab8d5}label{display:grid;gap:.45rem;margin:1.25rem 0}input{padding:.8rem;border:1px solid #405275;border-radius:.5rem;background:#0b1020;color:inherit;font:inherit}button{border:0;border-radius:.6rem;padding:.8rem 1.1rem;background:#72d4ff;color:#07111f;font-weight:800;font:inherit;cursor:pointer}.secondary{background:#263a5a;color:#e9eefb}.error{color:#ff9b9b}@media(max-width:560px){header{align-items:flex-start;flex-direction:column}.summary{flex-direction:column}}
</style>
