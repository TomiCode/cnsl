import { createRouter, createWebHistory } from 'vue-router'
import OverviewView from './views/OverviewView.vue'
import PlaceholderView from './views/PlaceholderView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'overview', component: OverviewView },
    { path: '/devices', name: 'devices', component: PlaceholderView, meta: { title: 'Devices', phase: 1 } },
    { path: '/screens', name: 'screens', component: PlaceholderView, meta: { title: 'Screens', phase: 2 } },
    { path: '/playlists', name: 'playlists', component: PlaceholderView, meta: { title: 'Playlists', phase: 2 } },
    { path: '/connectors', name: 'connectors', component: PlaceholderView, meta: { title: 'Connectors', phase: 4 } },
    { path: '/settings', name: 'settings', component: PlaceholderView, meta: { title: 'Settings', phase: 1 } },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: PlaceholderView, meta: { title: 'Not found' } },
  ],
})
