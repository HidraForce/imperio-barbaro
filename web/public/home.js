const canonicalOrigin = 'https://imperiobarbaro.com.br'
function openLinkedRule() {
  let id
  try { id = decodeURIComponent(location.hash.slice(1)) } catch { return }
  const target = document.getElementById(id)
  if (!target) return
  if (target.matches('details.rule')) target.open = true
  if (id.startsWith('regras-')) requestAnimationFrame(() => target.scrollIntoView({block:'start'}))
}
window.addEventListener('hashchange', openLinkedRule)
openLinkedRule()

const feedback = document.querySelector('.share-feedback')
const shareStatus = document.getElementById('share-status')
const shareURL = document.getElementById('share-url')
let copySequence = 0
document.querySelectorAll('[data-copy-rule]').forEach(button => button.addEventListener('click', async () => {
  const sequence = ++copySequence
  const url = `${canonicalOrigin}/#${button.dataset.copyRule}`
  shareURL.value = url
  shareURL.hidden = true
  feedback.hidden = false
  shareStatus.textContent = 'Copiando link…'
  try {
    await navigator.clipboard.writeText(url)
    if (sequence === copySequence) shareStatus.textContent = 'Link copiado. Envie para o novo membro.'
  } catch {
    if (sequence !== copySequence) return
    shareStatus.textContent = 'Copie o link abaixo para compartilhar as regras.'
    shareURL.hidden = false
    shareURL.focus()
    shareURL.select()
  }
}))
document.getElementById('share-dismiss').addEventListener('click', () => { copySequence++; feedback.hidden = true })

const status = document.getElementById('leadership-status')
const retry = document.getElementById('leadership-retry')
function element(tag, className, text) {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}
async function loadLeadership() {
  retry.hidden = true
  status.textContent = 'Carregando liderança atual…'
  try {
    const response = await fetch('/public/leadership', {credentials:'omit', cache:'no-store', signal:AbortSignal.timeout(10000)})
    if (!response.ok) throw new Error('unavailable')
    const data = await response.json()
    if (!Array.isArray(data.items) || !data.items.every(item => ['1','2'].includes(item.clanId) && ['role','name','description'].every(key => typeof item[key] === 'string'))) throw new Error('invalid data')
    document.querySelectorAll('[data-team-clan]').forEach(container => {
      const entries = data.items.filter(item => item.clanId === container.dataset.teamClan)
      const nodes = entries.map(item => {
        const article = element('article', 'team-person')
        const heading = element('div')
        heading.append(element('span', 'role', item.role), element('h4', item.name ? '' : 'vacant', item.name || 'Vaga aberta'))
        article.append(heading, element('p', '', item.description))
        return article
      })
      container.replaceChildren(...(nodes.length ? nodes : [element('p','team-loading','Nenhum cargo publicado para este clã.')]))
    })
    document.querySelectorAll('[data-leader-clan]').forEach(node => {
      const leader = data.items.find(item => item.clanId === node.dataset.leaderClan && item.role.normalize('NFD').replace(/[\u0300-\u036f]/g,'').trim().toLowerCase() === 'lider')
      node.textContent = leader ? leader.name || 'Vaga aberta' : 'Consulte o guia'
    })
    status.textContent = ''
  } catch {
    document.querySelectorAll('[data-team-clan]').forEach(container => container.replaceChildren())
    status.textContent = 'Não foi possível carregar a liderança atual. Tente novamente em instantes.'
    retry.hidden = false
  }
}
retry.addEventListener('click', loadLeadership)
loadLeadership()
