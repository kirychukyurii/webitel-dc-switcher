const DARK_MODE_LOCAL_STORAGE_KEY = 'darkMode'

const state = {
  darkMode: localStorage.getItem(DARK_MODE_LOCAL_STORAGE_KEY) === 'true' || false,
}

const getters = {
  DARK_MODE: (state) => state.darkMode,
}

const actions = {
  TOGGLE_DARK_MODE: (context) => {
    context.commit('SET_DARK_MODE', !context.state.darkMode)
  },
  SET_DARK_MODE: (context, value) => {
    context.commit('SET_DARK_MODE', value)
  },
}

const mutations = {
  SET_DARK_MODE: (state, value) => {
    state.darkMode = value
    localStorage.setItem(DARK_MODE_LOCAL_STORAGE_KEY, value)

    // Apply dark mode class to document
    if (value) {
      document.documentElement.classList.add('wt-dark-mode')
    } else {
      document.documentElement.classList.remove('wt-dark-mode')
    }
  },
}

export default {
  namespaced: true,
  state,
  getters,
  actions,
  mutations,
}
