<script setup>
import { ref, onMounted, computed } from 'vue'
import { useCollectionStore } from '../stores/collection'
import CardGrid from '../components/CardGrid.vue'
import CardDetail from '../components/CardDetail.vue'

const store = useCollectionStore()

const selectedItem = ref(null)
const filterGame = ref('all')
const sortBy = ref('added_at')
const searchQuery = ref('')
const refreshMessage = ref(null)
const refreshing = ref(false)

// Use grouped items for display with search filtering
const filteredItems = computed(() => {
  let items = [...store.groupedItems]

  // Game filter
  if (filterGame.value !== 'all') {
    items = items.filter(item => item.card.game === filterGame.value)
  }

  // Search filter (name, set)
  if (searchQuery.value.trim()) {
    const query = searchQuery.value.toLowerCase().trim()
    items = items.filter(item => {
      const card = item.card
      return (
        card.name?.toLowerCase().includes(query) ||
        card.set_name?.toLowerCase().includes(query) ||
        card.set_code?.toLowerCase().includes(query)
      )
    })
  }

  // Sorting
  items.sort((a, b) => {
    switch (sortBy.value) {
      case 'name':
        return a.card.name.localeCompare(b.card.name)
      case 'value':
        return (b.total_value || 0) - (a.total_value || 0)
      case 'price_updated': {
        // Sort by most recently updated price
        const priceA = a.card.price_updated_at ? new Date(a.card.price_updated_at) : new Date(0)
        const priceB = b.card.price_updated_at ? new Date(b.card.price_updated_at) : new Date(0)
        return priceB - priceA
      }
      case 'added_at':
      default: {
        // For grouped items, use the most recent item's added_at
        const latestA = a.items?.[0]?.added_at || ''
        const latestB = b.items?.[0]?.added_at || ''
        return new Date(latestB) - new Date(latestA)
      }
    }
  })

  return items
})

const handleSelect = (groupedItem) => {
  selectedItem.value = groupedItem
}

const handleUpdate = async ({ id, quantity, condition, printing }) => {
  const result = await store.updateItem(id, { quantity, condition, printing })
  // Show feedback about the operation
  if (result.message) {
    // Could show a toast here
    console.log('Update operation:', result.operation, result.message)
  }
  selectedItem.value = null
}

const handleRemove = async (id) => {
  await store.removeItem(id)
  selectedItem.value = null
}

const handleRefreshPrices = async () => {
  refreshing.value = true
  refreshMessage.value = null
  try {
    const result = await store.refreshPrices()
    if (result.queued > 0) {
      refreshMessage.value = {
        type: 'success',
        text: `Queued ${result.queued} cards for price update. Next batch in ~${Math.ceil((new Date(result.next_update_time) - new Date()) / 60000)} minutes.`
      }
    } else {
      refreshMessage.value = {
        type: 'info',
        text: 'No cards to refresh.'
      }
    }
    // Auto-hide message after 5 seconds
    setTimeout(() => { refreshMessage.value = null }, 5000)
  } catch (err) {
    refreshMessage.value = {
      type: 'error',
      text: err.message || 'Failed to queue price refresh'
    }
  } finally {
    refreshing.value = false
  }
}

const handlePriceUpdated = () => {
  // Refresh the collection to get updated data
  store.fetchGroupedCollection()
}

const handleClose = () => {
  selectedItem.value = null
}

onMounted(() => {
  store.fetchGroupedCollection()
  store.fetchStats()
})
</script>

<template>
  <div>
    <div class="flex flex-col gap-4 mb-6">
      <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <h1 class="text-3xl font-bold text-gray-800 dark:text-white">My Collection</h1>

        <div class="flex flex-wrap gap-3">
          <select
            v-model="filterGame"
            class="border dark:border-gray-600 rounded-lg px-3 py-2 bg-white dark:bg-gray-800 text-gray-900 dark:text-white"
          >
            <option value="all">All Games</option>
            <option value="mtg">Magic: The Gathering</option>
            <option value="pokemon">Pokemon</option>
          </select>

          <select
            v-model="sortBy"
            class="border dark:border-gray-600 rounded-lg px-3 py-2 bg-white dark:bg-gray-800 text-gray-900 dark:text-white"
          >
            <option value="added_at">Recently Added</option>
            <option value="name">Name</option>
            <option value="value">Value</option>
            <option value="price_updated">Price Updated</option>
          </select>

          <button
            @click="handleRefreshPrices"
            :disabled="refreshing"
            class="bg-green-600 text-white px-4 py-2 rounded-lg hover:bg-green-700 transition disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            <span v-if="refreshing" class="animate-spin h-4 w-4 border-2 border-white border-t-transparent rounded-full"></span>
            {{ refreshing ? 'Queueing...' : 'Refresh Prices' }}
          </button>
        </div>
      </div>

      <!-- Search Bar -->
      <div class="flex gap-3">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search by card name or set..."
          class="flex-1 border dark:border-gray-600 rounded-lg px-4 py-2 bg-white dark:bg-gray-800 text-gray-900 dark:text-white placeholder-gray-400"
        />
        <button
          v-if="searchQuery"
          @click="searchQuery = ''"
          class="px-3 py-2 text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
        >
          Clear
        </button>
      </div>

      <!-- Refresh Message Toast -->
      <div
        v-if="refreshMessage"
        :class="{
          'bg-green-100 dark:bg-green-900/30 text-green-800 dark:text-green-300 border-green-300 dark:border-green-700': refreshMessage.type === 'success',
          'bg-blue-100 dark:bg-blue-900/30 text-blue-800 dark:text-blue-300 border-blue-300 dark:border-blue-700': refreshMessage.type === 'info',
          'bg-red-100 dark:bg-red-900/30 text-red-800 dark:text-red-300 border-red-300 dark:border-red-700': refreshMessage.type === 'error'
        }"
        class="p-3 rounded-lg border text-sm"
      >
        {{ refreshMessage.text }}
      </div>
    </div>

    <div v-if="store.loading" class="text-center py-8">
      <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto"></div>
    </div>

    <div v-else-if="filteredItems.length === 0" class="text-center py-12 bg-white dark:bg-gray-800 rounded-lg">
      <p class="text-gray-500 dark:text-gray-400 mb-4">No cards found</p>
      <router-link
        to="/add"
        class="inline-block bg-blue-600 text-white px-6 py-2 rounded-lg hover:bg-blue-700"
      >
        Add Cards
      </router-link>
    </div>

    <CardGrid
      v-else
      :cards="filteredItems"
      :show-quantity="true"
      :grouped="true"
      @select="handleSelect"
    />

    <CardDetail
      v-if="selectedItem"
      :item="selectedItem"
      :is-collection-item="true"
      :is-grouped="true"
      @close="handleClose"
      @update="handleUpdate"
      @remove="handleRemove"
      @priceUpdated="handlePriceUpdated"
    />
  </div>
</template>
