import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      redirect: '/music',
    },
    {
      path: '/music',
      name: 'music',
      component: () => import('@/views/PlaylistView.vue'),
    },
    {
      path: '/search',
      name: 'search',
      component: () => import('@/views/SearchView.vue'),
    },
    {
      path: '/cast',
      name: 'cast',
      component: () => import('@/views/CastView.vue'),
    },
    {
      path: '/favlist',
      name: 'favlist',
      component: () => import('@/views/FavlistView.vue'),
    },
    {
      path: '/cache',
      name: 'cache',
      component: () => import('@/views/CacheView.vue'),
    },
  ],
})

export default router
