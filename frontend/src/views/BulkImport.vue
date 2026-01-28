<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useBulkImportStore } from '../stores/bulkImport'
import { useCollectionStore } from '../stores/collection'

const store = useBulkImportStore()
const collectionStore = useCollectionStore()

// Local state
const dragActive = ref(false)
const selectedFiles = ref([])
const selectedFilesPreviews = ref([])
const bulkCondition = ref('NM')
const bulkPrinting = ref('Normal')
const confirmMessage = ref(null)
const searchQuery = ref('')
const searchGame = ref('pokemon')
const editingItem = ref(null)

// Condition options
const conditions = [
  { value: 'NM', label: 'Near Mint' },
  { value: 'LP', label: 'Lightly Played' },
  { value: 'MP', label: 'Moderately Played' },
  { value: 'HP', label: 'Heavily Played' },
  { value: 'DMG', label: 'Damaged' }
]

// Printing options
const printings = [
  { value: 'Normal', label: 'Normal' },
  { value: 'Foil', label: 'Foil' },
  { value: '1st Edition', label: '1st Edition' },
  { value: 'Reverse Holofoil', label: 'Reverse Holo' },
  { value: 'Unlimited', label: 'Unlimited' }
]

// Computed
const canStartImport = computed(() => selectedFiles.value.length > 0 && !store.uploading)
const showUpload = computed(() => store.phase === 'upload')
const showProgress = computed(() => store.phase === 'processing')
const showReview = computed(() => store.phase === 'review')

// Initialize store on mount
onMounted(async () => {
  await store.init()
  if (store.phase === 'processing' && !store.isComplete) {
    store.startPolling()
  }
})

// Cleanup on unmount
onUnmounted(() => {
  store.stopPolling()
  // Cleanup file previews
  selectedFilesPreviews.value.forEach(url => URL.revokeObjectURL(url))
})

// File handling
function handleDragEnter(e) {
  e.preventDefault()
  dragActive.value = true
}

function handleDragLeave(e) {
  e.preventDefault()
  dragActive.value = false
}

function handleDragOver(e) {
  e.preventDefault()
}

function handleDrop(e) {
  e.preventDefault()
  dragActive.value = false
  
  const files = Array.from(e.dataTransfer.files).filter(f => f.type.startsWith('image/'))
  addFiles(files)
}

function handleFileSelect(e) {
  const files = Array.from(e.target.files)
  addFiles(files)
  e.target.value = '' // Reset input
}

function addFiles(files) {
  // Filter to only images and limit to 200 total
  const imageFiles = files.filter(f => f.type.startsWith('image/'))
  const remaining = 200 - selectedFiles.value.length
  const toAdd = imageFiles.slice(0, remaining)
  
  // Add files and create previews
  toAdd.forEach(file => {
    selectedFiles.value.push(file)
    selectedFilesPreviews.value.push(URL.createObjectURL(file))
  })
}

function removeFile(index) {
  URL.revokeObjectURL(selectedFilesPreviews.value[index])
  selectedFiles.value.splice(index, 1)
  selectedFilesPreviews.value.splice(index, 1)
}

function clearFiles() {
  selectedFilesPreviews.value.forEach(url => URL.revokeObjectURL(url))
  selectedFiles.value = []
  selectedFilesPreviews.value = []
}

// Start import
async function startImport() {
  if (!canStartImport.value) return
  
  try {
    await store.createJob(selectedFiles.value)
    clearFiles()
  } catch (err) {
    console.error('Failed to start import:', err)
  }
}

// Apply bulk settings to all items
async function applyBulkCondition() {
  for (const item of store.identifiedItems) {
    if (item.condition !== bulkCondition.value) {
      await store.updateItem(item.id, { condition: bulkCondition.value })
    }
  }
}

async function applyBulkPrinting() {
  for (const item of store.identifiedItems) {
    if (item.printing_type !== bulkPrinting.value) {
      await store.updateItem(item.id, { printing_type: bulkPrinting.value })
    }
  }
}

// Update single item
async function updateItemField(item, field, value) {
  try {
    await store.updateItem(item.id, { [field]: value })
  } catch (err) {
    console.error('Failed to update item:', err)
  }
}

// Search for manual card selection
async function searchCards() {
  if (!searchQuery.value.trim()) return
  await store.searchCards(searchQuery.value, searchGame.value)
}

function selectSearchResult(card) {
  if (!editingItem.value) return
  store.updateItem(editingItem.value.id, { card_id: card.id })
  editingItem.value = null
  store.clearSearch()
  searchQuery.value = ''
}

function openCardSearch(item) {
  editingItem.value = item
  searchQuery.value = item.card_name || ''
  searchGame.value = item.game || 'pokemon'
}

function closeCardSearch() {
  editingItem.value = null
  store.clearSearch()
  searchQuery.value = ''
}

// Confirm items
async function confirmAll() {
  try {
    const result = await store.confirmItems()
    confirmMessage.value = `Added ${result.added} cards to collection`
    if (result.skipped > 0) {
      confirmMessage.value += ` (${result.skipped} skipped)`
    }
    // Refresh collection
    collectionStore.fetchGroupedCollection()
    collectionStore.fetchStats()
    
    // If all items are now skipped/confirmed, offer to start new import
    setTimeout(() => {
      confirmMessage.value = null
    }, 5000)
  } catch (err) {
    console.error('Failed to confirm items:', err)
  }
}

// Cancel/delete job
async function cancelImport() {
  if (confirm('Are you sure you want to cancel this import? All progress will be lost.')) {
    await store.deleteJob()
  }
}

// Start new import
function startNewImport() {
  store.clearJob()
}

// Get confidence color class
function getConfidenceClass(confidence) {
  if (confidence >= 0.9) return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
  if (confidence >= 0.7) return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200'
  return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
}

// Get status icon
function getStatusIcon(status) {
  switch (status) {
    case 'identified': return '✓'
    case 'failed': return '✗'
    case 'processing': return '⟳'
    case 'pending': return '⏳'
    case 'skipped': return '→'
    default: return '?'
  }
}

// Error code display configuration
// Maps backend error codes to user-friendly messages and suggestions
const errorCodeConfig = {
  no_card_visible: {
    icon: '🖼️',
    title: 'No card detected',
    suggestion: 'Make sure the card fills most of the frame and is clearly visible.'
  },
  image_quality: {
    icon: '📷',
    title: 'Image quality issue',
    suggestion: 'Try taking a clearer photo with better lighting and focus.'
  },
  no_match: {
    icon: '🔍',
    title: 'Card not found',
    suggestion: 'The card was detected but couldn\'t be matched. Use "Search manually" to find it.'
  },
  api_error: {
    icon: '🌐',
    title: 'Service error',
    suggestion: 'The identification service is temporarily unavailable. This item can be retried later.'
  },
  timeout: {
    icon: '⏱️',
    title: 'Timed out',
    suggestion: 'Identification took too long. Try again or search manually.'
  },
  file_error: {
    icon: '📁',
    title: 'File error',
    suggestion: 'Could not read the image file. Try re-uploading.'
  },
  service_unavailable: {
    icon: '🔧',
    title: 'Service not configured',
    suggestion: 'Card identification is not available. Contact the administrator.'
  }
}

// Get user-friendly error display info
function getErrorInfo(item) {
  const code = item.error_code || 'no_match'
  const config = errorCodeConfig[code] || errorCodeConfig.no_match
  return {
    ...config,
    details: item.error_message || ''
  }
}

// Get image URL for bulk import images
function getBulkImportImageUrl(imagePath) {
  if (!imagePath) return ''
  const baseUrl = import.meta.env.VITE_API_URL?.replace('/api', '') || ''
  return `${baseUrl}/images/bulk-import/${imagePath}`
}
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex justify-between items-center">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Bulk Import</h1>
      <div v-if="store.hasJob && !showUpload" class="flex gap-2">
        <button
          v-if="showReview && store.confirmableCount === 0"
          @click="startNewImport"
          class="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition"
        >
          Start New Import
        </button>
        <button
          @click="cancelImport"
          class="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition"
        >
          Cancel Import
        </button>
      </div>
    </div>

    <!-- Error message -->
    <div v-if="store.error" class="bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 p-4 rounded-lg">
      {{ store.error }}
      <button @click="store.clearError" class="ml-2 underline">Dismiss</button>
    </div>

    <!-- Success message -->
    <div v-if="confirmMessage" class="bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-200 p-4 rounded-lg">
      {{ confirmMessage }}
    </div>

    <!-- Phase 1: Upload -->
    <div v-if="showUpload" class="space-y-6">
      <!-- Drop zone -->
      <div
        class="border-2 border-dashed rounded-lg p-12 text-center transition-colors cursor-pointer"
        :class="{
          'border-blue-500 bg-blue-50 dark:bg-blue-900/20': dragActive,
          'border-gray-300 dark:border-gray-600 hover:border-blue-400': !dragActive
        }"
        @dragenter="handleDragEnter"
        @dragleave="handleDragLeave"
        @dragover="handleDragOver"
        @drop="handleDrop"
        @click="$refs.fileInput.click()"
      >
        <input
          ref="fileInput"
          type="file"
          accept="image/*"
          multiple
          class="hidden"
          @change="handleFileSelect"
        />
        <svg class="mx-auto h-16 w-16 text-gray-400 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
        </svg>
        <p class="text-lg text-gray-600 dark:text-gray-300 mb-2">
          Drag & drop images here, or click to browse
        </p>
        <p class="text-sm text-gray-500 dark:text-gray-400">
          Up to 200 images (JPG, PNG, WebP)
        </p>
      </div>

      <!-- Selected files preview -->
      <div v-if="selectedFiles.length > 0" class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-medium text-gray-900 dark:text-white">
            Selected: {{ selectedFiles.length }} images
          </h3>
          <button
            @click="clearFiles"
            class="text-red-600 dark:text-red-400 hover:underline"
          >
            Clear all
          </button>
        </div>
        
        <div class="grid grid-cols-6 sm:grid-cols-8 md:grid-cols-10 lg:grid-cols-12 gap-2">
          <div
            v-for="(preview, index) in selectedFilesPreviews"
            :key="index"
            class="relative aspect-square group"
          >
            <img
              :src="preview"
              :alt="selectedFiles[index].name"
              class="w-full h-full object-cover rounded"
            />
            <button
              @click.stop="removeFile(index)"
              class="absolute -top-1 -right-1 w-5 h-5 bg-red-500 text-white rounded-full text-xs opacity-0 group-hover:opacity-100 transition"
            >
              ×
            </button>
          </div>
        </div>

        <!-- Start import button -->
        <div class="flex justify-center">
          <button
            @click="startImport"
            :disabled="!canStartImport"
            class="px-8 py-3 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 transition disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <span v-if="store.uploading">
              Uploading... {{ store.uploadProgress }}%
            </span>
            <span v-else>
              Start Import ({{ selectedFiles.length }} images)
            </span>
          </button>
        </div>
      </div>
    </div>

    <!-- Phase 2: Processing -->
    <div v-if="showProgress" class="space-y-6">
      <!-- Resuming indicator -->
      <div v-if="store.resuming" class="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4 flex items-center gap-3">
        <svg class="animate-spin h-5 w-5 text-blue-600 dark:text-blue-400" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
        <span class="text-blue-700 dark:text-blue-300 font-medium">Resuming... Fetching latest progress</span>
      </div>
      
      <!-- Progress bar -->
      <div class="bg-white dark:bg-gray-800 rounded-lg p-6 shadow">
        <div class="flex justify-between items-center mb-2">
          <span class="text-lg font-medium text-gray-900 dark:text-white">
            Processing: {{ store.displayProcessed }} / {{ store.displayTotal }}
          </span>
          <span class="text-lg font-bold text-blue-600 dark:text-blue-400">
            {{ store.progress }}%
          </span>
        </div>
        <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-4">
          <div
            class="bg-blue-600 h-4 rounded-full transition-all duration-300"
            :style="{ width: `${store.progress}%` }"
          ></div>
        </div>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
          You can close this page. Your progress is saved and will resume when you return.
        </p>
      </div>

      <!-- Status summary -->
      <div v-if="!store.resuming" class="grid grid-cols-4 gap-4">
        <div class="bg-white dark:bg-gray-800 rounded-lg p-4 text-center shadow">
          <div class="text-2xl font-bold text-green-600">{{ store.identifiedCount }}</div>
          <div class="text-sm text-gray-500 dark:text-gray-400">Identified</div>
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-lg p-4 text-center shadow">
          <div class="text-2xl font-bold text-blue-600">{{ store.processingCount }}</div>
          <div class="text-sm text-gray-500 dark:text-gray-400">Processing</div>
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-lg p-4 text-center shadow">
          <div class="text-2xl font-bold text-gray-600">{{ store.pendingCount }}</div>
          <div class="text-sm text-gray-500 dark:text-gray-400">Pending</div>
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-lg p-4 text-center shadow">
          <div class="text-2xl font-bold text-amber-600">{{ store.failedCount }}</div>
          <div class="text-sm text-gray-500 dark:text-gray-400">Needs Attention</div>
        </div>
      </div>

      <!-- Item list -->
      <div v-if="!store.resuming && store.job?.items" class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
        <div class="max-h-96 overflow-y-auto">
          <div
            v-for="item in store.job?.items"
            :key="item.id"
            class="flex items-center gap-4 p-3 border-b dark:border-gray-700 last:border-b-0"
          >
            <!-- Status icon -->
            <span
              class="w-8 h-8 flex items-center justify-center rounded-full text-sm font-bold"
              :class="{
                'bg-green-100 text-green-600 dark:bg-green-900 dark:text-green-400': item.status === 'identified',
                'bg-red-100 text-red-600 dark:bg-red-900 dark:text-red-400': item.status === 'failed',
                'bg-blue-100 text-blue-600 dark:bg-blue-900 dark:text-blue-400 animate-spin': item.status === 'processing',
                'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400': item.status === 'pending'
              }"
            >
              {{ getStatusIcon(item.status) }}
            </span>
            
            <!-- Filename -->
            <span class="flex-1 text-sm text-gray-700 dark:text-gray-300 truncate">
              {{ item.original_filename }}
            </span>
            
            <!-- Result -->
            <span v-if="item.status === 'identified'" class="text-sm text-gray-900 dark:text-white font-medium">
              {{ item.card_name }}
            </span>
            <span v-else-if="item.status === 'failed'" class="text-sm text-amber-600 dark:text-amber-400 flex items-center gap-1">
              <span>{{ getErrorInfo(item).icon }}</span>
              <span>{{ getErrorInfo(item).title }}</span>
            </span>
            <span v-else class="text-sm text-gray-500 dark:text-gray-400">
              {{ item.status === 'processing' ? 'Identifying...' : 'Waiting...' }}
            </span>
            
            <!-- Confidence badge -->
            <span
              v-if="item.status === 'identified'"
              class="px-2 py-1 rounded text-xs font-medium"
              :class="getConfidenceClass(item.confidence)"
            >
              {{ Math.round(item.confidence * 100) }}%
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- Phase 3: Review -->
    <div v-if="showReview" class="space-y-6">
      <!-- Bulk actions -->
      <div class="bg-white dark:bg-gray-800 rounded-lg p-4 shadow flex flex-wrap gap-4 items-center">
        <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Set all:</span>
        
        <div class="flex items-center gap-2">
          <select
            v-model="bulkCondition"
            class="border dark:border-gray-600 rounded px-3 py-1.5 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm"
          >
            <option v-for="c in conditions" :key="c.value" :value="c.value">{{ c.label }}</option>
          </select>
          <button
            @click="applyBulkCondition"
            class="px-3 py-1.5 bg-gray-200 dark:bg-gray-600 text-gray-700 dark:text-gray-200 rounded text-sm hover:bg-gray-300 dark:hover:bg-gray-500"
          >
            Apply
          </button>
        </div>
        
        <div class="flex items-center gap-2">
          <select
            v-model="bulkPrinting"
            class="border dark:border-gray-600 rounded px-3 py-1.5 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm"
          >
            <option v-for="p in printings" :key="p.value" :value="p.value">{{ p.label }}</option>
          </select>
          <button
            @click="applyBulkPrinting"
            class="px-3 py-1.5 bg-gray-200 dark:bg-gray-600 text-gray-700 dark:text-gray-200 rounded text-sm hover:bg-gray-300 dark:hover:bg-gray-500"
          >
            Apply
          </button>
        </div>
        
        <div class="flex-1"></div>
        
        <button
          v-if="store.confirmableCount > 0"
          @click="confirmAll"
          :disabled="store.loading"
          class="px-6 py-2 bg-green-600 text-white rounded-lg font-medium hover:bg-green-700 transition disabled:opacity-50"
        >
          Add {{ store.confirmableCount }} Cards to Collection
        </button>
      </div>

      <!-- Card search modal -->
      <div
        v-if="editingItem"
        class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
        @click.self="closeCardSearch"
      >
        <div class="bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-2xl w-full max-h-[80vh] overflow-hidden">
          <div class="p-4 border-b dark:border-gray-700">
            <div class="flex justify-between items-center mb-4">
              <h3 class="text-lg font-medium text-gray-900 dark:text-white">Select Card</h3>
              <button @click="closeCardSearch" class="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200">
                <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            
            <div class="flex gap-2">
              <select
                v-model="searchGame"
                class="border dark:border-gray-600 rounded px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
              >
                <option value="pokemon">Pokemon</option>
                <option value="mtg">Magic: The Gathering</option>
              </select>
              <input
                v-model="searchQuery"
                type="text"
                placeholder="Search card name..."
                class="flex-1 border dark:border-gray-600 rounded px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                @keyup.enter="searchCards"
              />
              <button
                @click="searchCards"
                :disabled="store.searchLoading"
                class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 transition disabled:opacity-50"
              >
                Search
              </button>
            </div>
          </div>
          
          <!-- Candidates from Gemini -->
          <div v-if="editingItem.candidate_list?.length > 0" class="p-4 border-b dark:border-gray-700">
            <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Gemini Suggestions:</h4>
            <div class="grid grid-cols-4 gap-2 max-h-40 overflow-y-auto">
              <button
                v-for="card in editingItem.candidate_list"
                :key="card.id"
                @click="selectSearchResult(card)"
                class="p-2 border dark:border-gray-600 rounded hover:bg-blue-50 dark:hover:bg-blue-900/20 text-left"
              >
                <img v-if="card.image_url" :src="card.image_url" :alt="card.name" class="w-full h-24 object-contain mb-1" />
                <div class="text-xs font-medium text-gray-900 dark:text-white truncate">{{ card.name }}</div>
                <div class="text-xs text-gray-500 dark:text-gray-400 truncate">{{ card.set_name }}</div>
              </button>
            </div>
          </div>
          
          <!-- Search results -->
          <div class="p-4 max-h-64 overflow-y-auto">
            <div v-if="store.searchLoading" class="text-center py-4 text-gray-500">
              Searching...
            </div>
            <div v-else-if="store.searchResults.length === 0" class="text-center py-4 text-gray-500">
              {{ searchQuery ? 'No results found' : 'Enter a search term' }}
            </div>
            <div v-else class="grid grid-cols-4 gap-2">
              <button
                v-for="card in store.searchResults"
                :key="card.id"
                @click="selectSearchResult(card)"
                class="p-2 border dark:border-gray-600 rounded hover:bg-blue-50 dark:hover:bg-blue-900/20 text-left"
              >
                <img v-if="card.image_url" :src="card.image_url" :alt="card.name" class="w-full h-24 object-contain mb-1" />
                <div class="text-xs font-medium text-gray-900 dark:text-white truncate">{{ card.name }}</div>
                <div class="text-xs text-gray-500 dark:text-gray-400 truncate">{{ card.set_name }}</div>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Identified cards grid -->
      <div v-if="store.identifiedItems.length > 0" class="space-y-4">
        <h3 class="text-lg font-medium text-gray-900 dark:text-white">
          Identified Cards ({{ store.identifiedCount }})
        </h3>
        
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <div
            v-for="item in store.identifiedItems"
            :key="item.id"
            class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden"
          >
            <div class="flex gap-3 p-3">
              <!-- Scanned image -->
              <div class="w-20 flex-shrink-0">
                <img
                  v-if="item.image_path"
                  :src="getBulkImportImageUrl(item.image_path)"
                  alt="Scanned"
                  class="w-full h-28 object-cover rounded"
                />
              </div>
              
              <!-- Card image -->
              <div class="w-20 flex-shrink-0">
                <img
                  v-if="item.card?.image_url"
                  :src="item.card.image_url"
                  :alt="item.card_name"
                  class="w-full h-28 object-contain"
                />
              </div>
              
              <!-- Card info -->
              <div class="flex-1 min-w-0">
                <div class="flex items-start justify-between gap-2">
                  <div class="min-w-0">
                    <h4 class="font-medium text-gray-900 dark:text-white truncate">
                      {{ item.card_name }}
                    </h4>
                    <p class="text-sm text-gray-500 dark:text-gray-400 truncate">
                      {{ item.set_name }} #{{ item.card_number }}
                    </p>
                  </div>
                  <span
                    class="px-2 py-0.5 rounded text-xs font-medium flex-shrink-0"
                    :class="getConfidenceClass(item.confidence)"
                  >
                    {{ Math.round(item.confidence * 100) }}%
                  </span>
                </div>
                
                <!-- Controls -->
                <div class="mt-2 space-y-2">
                  <div class="flex gap-2">
                    <select
                      :value="item.condition"
                      @change="updateItemField(item, 'condition', $event.target.value)"
                      class="text-xs border dark:border-gray-600 rounded px-2 py-1 bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                    >
                      <option v-for="c in conditions" :key="c.value" :value="c.value">{{ c.label }}</option>
                    </select>
                    
                    <select
                      :value="item.printing_type"
                      @change="updateItemField(item, 'printing_type', $event.target.value)"
                      class="text-xs border dark:border-gray-600 rounded px-2 py-1 bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                    >
                      <option v-for="p in printings" :key="p.value" :value="p.value">{{ p.label }}</option>
                    </select>
                  </div>
                  
                  <button
                    @click="openCardSearch(item)"
                    class="text-xs text-blue-600 dark:text-blue-400 hover:underline"
                  >
                    Select different card
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Failed cards -->
      <div v-if="store.failedItems.length > 0" class="space-y-4">
        <h3 class="text-lg font-medium text-red-600 dark:text-red-400">
          Needs Attention ({{ store.failedCount }})
        </h3>
        
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <div
            v-for="item in store.failedItems"
            :key="item.id"
            class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden border-l-4 border-amber-500"
          >
            <div class="flex gap-3 p-3">
              <!-- Scanned image -->
              <div class="w-20 flex-shrink-0">
                <img
                  v-if="item.image_path"
                  :src="getBulkImportImageUrl(item.image_path)"
                  alt="Scanned"
                  class="w-full h-28 object-cover rounded"
                />
              </div>
              
              <!-- Error info with icon and suggestion -->
              <div class="flex-1 min-w-0">
                <p class="text-sm text-gray-900 dark:text-white font-medium truncate">
                  {{ item.original_filename }}
                </p>
                
                <!-- Error category with icon -->
                <div class="flex items-center gap-1.5 mt-1">
                  <span class="text-base">{{ getErrorInfo(item).icon }}</span>
                  <span class="text-sm font-medium text-amber-700 dark:text-amber-400">
                    {{ getErrorInfo(item).title }}
                  </span>
                </div>
                
                <!-- Suggestion -->
                <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
                  {{ getErrorInfo(item).suggestion }}
                </p>
                
                <!-- Show details toggle for debugging -->
                <details v-if="getErrorInfo(item).details" class="mt-1">
                  <summary class="text-xs text-gray-400 dark:text-gray-500 cursor-pointer hover:text-gray-600 dark:hover:text-gray-300">
                    Show details
                  </summary>
                  <p class="text-xs text-gray-400 dark:text-gray-500 mt-0.5 break-words">
                    {{ getErrorInfo(item).details }}
                  </p>
                </details>
                
                <button
                  @click="openCardSearch(item)"
                  class="mt-2 text-sm text-blue-600 dark:text-blue-400 hover:underline font-medium"
                >
                  Search manually
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
