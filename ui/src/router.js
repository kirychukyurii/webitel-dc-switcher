import { createRouter, createWebHistory } from 'vue-router'
import RegionsView from './views/RegionsView.vue'
import DatacenterDetailView from './views/DatacenterDetailView.vue'

const routes = [
  {
    path: '/',
    redirect: '/regions',
  },
  {
    path: '/regions',
    name: 'Regions',
    component: RegionsView,
  },
  {
    path: '/datacenter/:name',
    name: 'DatacenterDetail',
    component: DatacenterDetailView,
    props: true,
  },
]

// Get base path from global variable injected by backend
const basePath = window._BASE_PATH || '/'

const router = createRouter({
  history: createWebHistory(basePath),
  routes,
})

export default router
