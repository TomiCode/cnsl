import { createRouter, createWebHistory } from 'vue-router'
import OverviewView from './views/OverviewView.vue'
import PlaceholderView from './views/PlaceholderView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'overview', component: OverviewView },
    { path: '/devices', name: 'devices', component: () => import('./views/DevicesView.vue') },
    { path: '/devices/:id(\\d+)', name: 'device', component: () => import('./views/DeviceView.vue') },
    { path: '/virtual', name: 'virtual', component: () => import('./views/VirtualDeviceView.vue') },
    { path: '/screens', name: 'screens', component: PlaceholderView, meta: { title: 'Screens', phase: 2 } },
    { path: '/playlists', name: 'playlists', component: PlaceholderView, meta: { title: 'Playlists', phase: 2 } },
    { path: '/connectors', name: 'connectors', component: PlaceholderView, meta: { title: 'Connectors', phase: 4 } },
    { path: '/settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: PlaceholderView, meta: { title: 'Not found' } },
  ],
})
