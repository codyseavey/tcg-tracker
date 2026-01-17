import { createRouter, createWebHistory } from 'vue-router'

// Lazy load views for better initial load performance
const Dashboard = () => import('./views/Dashboard.vue')
const Collection = () => import('./views/Collection.vue')
const AddCard = () => import('./views/AddCard.vue')
const NotFound = () => import('./views/NotFound.vue')

const routes = [
  {
    path: '/',
    name: 'Dashboard',
    component: Dashboard,
    meta: { title: 'Dashboard' }
  },
  {
    path: '/collection',
    name: 'Collection',
    component: Collection,
    meta: { title: 'Collection' }
  },
  {
    path: '/add',
    name: 'AddCard',
    component: AddCard,
    meta: { title: 'Add Card' }
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: NotFound,
    meta: { title: 'Page Not Found' }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// Update document title on route change
router.beforeEach((to, from, next) => {
  document.title = to.meta.title ? `${to.meta.title} - TCG Tracker` : 'TCG Tracker'
  next()
})

export default router
