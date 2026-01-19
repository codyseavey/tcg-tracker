<script setup>
import { ref, onMounted } from 'vue'
import { useAuthStore } from '../stores/auth'

const authStore = useAuthStore()
const keyInput = ref('')
const inputRef = ref(null)

onMounted(() => {
  // Focus the input when modal opens
  inputRef.value?.focus()
})

const handleSubmit = async () => {
  if (!keyInput.value.trim()) {
    authStore.error = 'Please enter an admin key'
    return
  }

  const success = await authStore.verifyKey(keyInput.value.trim())
  if (success) {
    keyInput.value = ''
    await authStore.retryPendingAction()
  }
}

const handleClose = () => {
  keyInput.value = ''
  authStore.closeAuthModal()
}

const handleKeydown = (e) => {
  if (e.key === 'Escape') {
    handleClose()
  }
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="authStore.showAuthModal"
      class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4"
      @click.self="handleClose"
      @keydown="handleKeydown"
      role="dialog"
      aria-modal="true"
      aria-labelledby="auth-modal-title"
    >
      <div class="bg-white dark:bg-gray-800 rounded-lg max-w-md w-full p-6 shadow-xl">
        <div class="flex justify-between items-start mb-4">
          <div>
            <h2 id="auth-modal-title" class="text-xl font-bold text-gray-800 dark:text-white">
              Admin Access Required
            </h2>
            <p class="text-gray-500 dark:text-gray-400 text-sm mt-1">
              Enter the admin key to modify the collection
            </p>
          </div>
          <button
            @click="handleClose"
            class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
            aria-label="Close"
          >
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <form @submit.prevent="handleSubmit" class="space-y-4">
          <div>
            <label for="admin-key" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Admin Key
            </label>
            <input
              ref="inputRef"
              id="admin-key"
              v-model="keyInput"
              type="password"
              placeholder="Enter admin key"
              class="w-full border dark:border-gray-600 rounded-lg px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              :disabled="authStore.verifying"
              autocomplete="current-password"
            />
          </div>

          <div v-if="authStore.error" class="text-red-500 text-sm">
            {{ authStore.error }}
          </div>

          <div class="flex gap-3">
            <button
              type="button"
              @click="handleClose"
              class="flex-1 px-4 py-2 border dark:border-gray-600 rounded-lg text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              :disabled="authStore.verifying || !keyInput.trim()"
              class="flex-1 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <span v-if="authStore.verifying">Verifying...</span>
              <span v-else>Unlock</span>
            </button>
          </div>
        </form>

        <div class="mt-4 pt-4 border-t dark:border-gray-700">
          <p class="text-xs text-gray-500 dark:text-gray-400">
            The key will be saved in your browser for future sessions.
            You can clear it from the lock icon in the navigation.
          </p>
        </div>
      </div>
    </div>
  </Teleport>
</template>
