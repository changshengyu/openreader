import { createRouter, createWebHistory } from 'vue-router'

const Discover = () => import('../views/Discover.vue')
const Home = () => import('../views/Home.vue')
const Login = () => import('../views/Login.vue')
const Reader = () => import('../views/Reader.vue')
const LocalStore = () => import('../views/LocalStore.vue')
const Search = () => import('../views/Search.vue')
const Settings = () => import('../views/Settings.vue')
const Sources = () => import('../views/Sources.vue')

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: Home },
    { path: '/login', name: 'login', component: Login },
    { path: '/search', name: 'search', component: Search },
    { path: '/discover', name: 'discover', component: Discover },
    { path: '/local-store', name: 'local-store', component: LocalStore },
    { path: '/sources', name: 'sources', component: Sources },
    { path: '/settings', name: 'settings', component: Settings },
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
  ],
})

router.beforeEach((to) => {
  const token = localStorage.getItem('openreader_token')
  if (!token && to.name !== 'login') {
    return { name: 'login' }
  }
  if (token && to.name === 'login') {
    return { name: 'home' }
  }
  return true
})

export default router
