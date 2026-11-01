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
    { path: '/screens', name: 'screens', component: () => import('./views/ScreensView.vue') },
    { path: '/screens/:id(\\d+)', name: 'screen', component: () => import('./views/ScreenEditorView.vue') },
    { path: '/playlists', name: 'playlists', component: () => import('./views/PlaylistsView.vue') },
    { path: '/images', name: 'images', component: () => import('./views/AssetsView.vue') },
    { path: '/connectors', name: 'connectors', component: () => import('./views/ConnectorsView.vue') },
    { path: '/firmware', name: 'firmware', component: () => import('./views/FirmwareView.vue') },
    { path: '/library', name: 'library', component: () => import('./views/LibraryView.vue') },
    { path: '/settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: PlaceholderView, meta: { title: 'Not found' } },
  ],
})
