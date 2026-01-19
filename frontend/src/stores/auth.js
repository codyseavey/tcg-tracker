import { defineStore } from 'pinia'
import api from '../services/api'

const STORAGE_KEY = 'tcg-admin-key'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    adminKey: localStorage.getItem(STORAGE_KEY) || null,
    authEnabled: null, // null = unknown, true/false = checked
    showAuthModal: false,
    pendingAction: null, // callback to retry after auth
    verifying: false,
    error: null
  }),

  getters: {
    isAuthenticated: (state) => !!state.adminKey,
    // Auth is required if enabled and user doesn't have a key
    requiresAuth: (state) => state.authEnabled === true && !state.adminKey
  },

  actions: {
    /**
     * Check if authentication is enabled on the server
     */
    async checkAuthStatus() {
      try {
        const response = await api.get('/auth/status')
        this.authEnabled = response.data.auth_enabled
      } catch {
        // If we can't check, assume auth might be enabled
        this.authEnabled = true
      }
    },

    /**
     * Verify the admin key with the server
     */
    async verifyKey(key) {
      this.verifying = true
      this.error = null

      try {
        const response = await api.post('/auth/verify', null, {
          headers: { Authorization: `Bearer ${key}` }
        })

        if (response.data.valid) {
          this.setAdminKey(key)
          return true
        } else {
          this.error = 'Invalid admin key'
          return false
        }
      } catch (err) {
        if (err.response?.status === 401) {
          this.error = err.response.data?.error || 'Invalid admin key'
        } else {
          this.error = 'Failed to verify key'
        }
        return false
      } finally {
        this.verifying = false
      }
    },

    /**
     * Store the admin key
     */
    setAdminKey(key) {
      this.adminKey = key
      localStorage.setItem(STORAGE_KEY, key)
      this.error = null
    },

    /**
     * Clear the stored admin key
     */
    clearAdminKey() {
      this.adminKey = null
      localStorage.removeItem(STORAGE_KEY)
    },

    /**
     * Show the auth modal, optionally with a pending action to retry
     */
    promptForAuth(pendingAction = null) {
      this.pendingAction = pendingAction
      this.showAuthModal = true
      this.error = null
    },

    /**
     * Close the auth modal
     */
    closeAuthModal() {
      this.showAuthModal = false
      this.pendingAction = null
      this.error = null
    },

    /**
     * Called after successful authentication to retry the pending action
     */
    async retryPendingAction() {
      if (this.pendingAction) {
        const action = this.pendingAction
        this.pendingAction = null
        this.showAuthModal = false
        try {
          await action()
        } catch {
          // Action might fail for other reasons, that's ok
        }
      } else {
        this.showAuthModal = false
      }
    },

    /**
     * Handle a 401 error from the API
     */
    handleAuthError(pendingAction = null) {
      // Clear potentially invalid key
      this.clearAdminKey()
      this.promptForAuth(pendingAction)
    }
  }
})
