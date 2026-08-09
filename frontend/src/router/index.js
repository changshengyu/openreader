import { createRouter, createWebHistory } from 'vue-router'
import { safeReturnTo } from '../utils/authNavigation'

const Home = () => import('../views/Home.vue')
const Login = () => import('../views/Login.vue')
const Reader = () => import('../views/Reader.vue')
const SourceDebug = () => import('../views/SourceDebug.vue')

function sourceOverlayIntentFromLegacy(to) {
  if (to.query.panel === 'remote') return 'remote'
  if (['import', 'health'].includes(to.query.action)) return to.query.action
  return 'manage'
}

function workspaceOverlayIntentFromLegacy(to, kind) {
  const { panel, ...query } = to.query
  if (kind === 'local-store') {
    return {
      path: '/',
      query: { ...query, overlay: 'local-store' },
    }
  }
  const settingsPanel = ['account', 'backup', 'cache', 'webdav', 'reader', 'replace', 'rss', 'admin'].includes(panel)
    ? panel
    : 'account'
  const overlayByPanel = {
    backup: 'webdav',
    webdav: 'webdav',
    replace: 'replace-rules',
    rss: 'rss',
    admin: 'user-manage',
  }
  const workspaceFocusByPanel = {
    account: 'account',
    cache: 'cache',
  }
  const overlay = overlayByPanel[settingsPanel]
  return {
    path: '/',
    query: {
      ...query,
      ...(overlay ? { overlay } : {}),
      ...(workspaceFocusByPanel[settingsPanel] ? { workspaceFocus: workspaceFocusByPanel[settingsPanel] } : {}),
      ...(settingsPanel === 'reader' ? { workspaceNotice: 'reader-settings' } : {}),
    },
  }
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: Home },
    { path: '/login', name: 'login', component: Login },
    {
      path: '/search',
      name: 'search',
      redirect: to => ({
        path: '/',
        query: {
          ...to.query,
          workspace: 'search',
        },
      }),
    },
    {
      path: '/discover',
      name: 'discover',
      redirect: to => ({
        path: '/',
        query: {
          ...to.query,
          workspace: 'explore',
        },
      }),
    },
    {
      path: '/local-store',
      name: 'local-store',
      redirect: to => workspaceOverlayIntentFromLegacy(to, 'local-store'),
    },
    {
      path: '/sources',
      name: 'sources',
      redirect: to => {
        const { panel, action, ...query } = to.query
        if (action === 'debug') {
          return { name: 'source-debug', query }
        }
        return {
          path: '/',
          query: {
            ...query,
            overlay: 'sources',
            sourceAction: sourceOverlayIntentFromLegacy(to),
          },
        }
      },
    },
    {
      path: '/source-debug',
      alias: ['/bookSourceDebug', '/bookSourceDebug/'],
      name: 'source-debug',
      component: SourceDebug,
    },
    {
      path: '/settings',
      name: 'settings',
      redirect: to => workspaceOverlayIntentFromLegacy(to, 'settings'),
    },
    {
      path: '/books/:id',
      name: 'book-detail',
      redirect: to => ({
        path: '/',
        query: {
          ...to.query,
          bookInfo: to.params.id,
        },
      }),
    },
    { path: '/books/:id/read', name: 'reader', component: Reader },
    { path: '/reader/remote/:sessionId', name: 'remote-reader', component: Reader },
  ],
})

router.beforeEach((to) => {
  const token = localStorage.getItem('openreader_token')
  if (!token && to.name !== 'login') {
    return {
      name: 'login',
      query: { returnTo: safeReturnTo(to.fullPath) },
    }
  }
  if (token && to.name === 'login') {
    return { name: 'home' }
  }
  return true
})

export default router
