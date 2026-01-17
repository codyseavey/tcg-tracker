import { defineStore } from 'pinia'

const STORAGE_KEY = 'tcg-tracker-theme'

export const useThemeStore = defineStore('theme', {
  state: () => ({
    currentTheme: localStorage.getItem(STORAGE_KEY) || 'system'
  }),

  getters: {
    isDark: (state) => {
      if (state.currentTheme === 'dark') return true
      if (state.currentTheme === 'light') return false
      // System preference
      return window.matchMedia('(prefers-color-scheme: dark)').matches
    }
  },

  actions: {
    setTheme(theme) {
      this.currentTheme = theme
      localStorage.setItem(STORAGE_KEY, theme)
      this.applyTheme()
    },

    applyTheme() {
      const isDark = this.isDark
      if (isDark) {
        document.documentElement.classList.add('dark')
      } else {
        document.documentElement.classList.remove('dark')
      }
    },

    initTheme() {
      this.applyTheme()
      // Listen for system theme changes
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
        if (this.currentTheme === 'system') {
          this.applyTheme()
        }
      })
    }
  }
})
