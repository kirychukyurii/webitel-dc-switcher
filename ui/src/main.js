import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import mitt from 'mitt'
import router from './router'
import store from './store'
import App from './App.vue'

// Import Webitel UI SDK components and styles
import WebitelUI from '@webitel/ui-sdk'
import '@webitel/ui-sdk/dist/ui-sdk.css'

// Create i18n instance for Webitel UI SDK
const i18n = createI18n({
  locale: 'en',
  fallbackLocale: 'en',
  legacy: false,
  messages: {
    en: {},
  },
})

// Create event bus for Webitel UI SDK (wrap mitt with $ methods)
const emitter = mitt()
const eventBus = {
  $on: (...args) => emitter.on(...args),
  $off: (...args) => emitter.off(...args),
  $emit: (...args) => emitter.emit(...args),
}

const app = createApp(App)

app.use(createPinia())
app.use(store)
app.use(i18n)
app.use(router)
app.use(WebitelUI, {
  eventBus,
  router,
})

// Initialize dark mode on app mount
const darkMode = store.state.appearance.darkMode
if (darkMode) {
  document.documentElement.classList.add('wt-dark-mode')
}

app.mount('#app')
