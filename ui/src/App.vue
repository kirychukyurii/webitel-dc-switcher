<template>
  <div class="dc-switcher">
    <wt-app-header>
      <wt-logo
        :dark-mode="darkMode"
        logo-href="/"
      />
      <div class="header-spacer" />
    </wt-app-header>

    <status-bar />

    <main class="app-content">
      <router-view />
    </main>

    <wt-notifications-bar />
  </div>
</template>

<script>
import { ref } from 'vue'
import StatusBar from './components/StatusBar.vue'

export default {
  name: 'App',
  components: {
    StatusBar,
  },
  setup() {
    const darkMode = ref(false)

    return {
      darkMode,
    }
  },
}
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
}

.dc-switcher {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background-color: #f5f7fa;
}

.header-spacer {
  flex: 1;
}

.app-content {
  flex: 1;
  max-width: 1400px;
  margin: 0 auto;
  width: 100%;
  padding: 24px;
}
</style>
