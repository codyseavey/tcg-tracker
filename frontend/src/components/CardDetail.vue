<script setup>
import { ref, computed } from 'vue'
import { priceService } from '../services/api'
import { formatPrice, formatTimeAgo, isPriceStale as checkPriceStale } from '../utils/formatters'

const props = defineProps({
  item: {
    type: Object,
    required: true
  },
  isCollectionItem: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['close', 'add', 'update', 'remove', 'priceUpdated'])

const card = computed(() => props.item.card || props.item)
const quantity = ref(props.item.quantity || 1)
const condition = ref(props.item.condition || 'NM')
const foil = ref(props.item.foil || false)
const firstEdition = ref(props.item.first_edition || false)
const refreshingPrice = ref(false)
const priceError = ref(null)
const showScannedImage = ref(false)

const hasScannedImage = computed(() => props.item.scanned_image_path)
const scannedImageUrl = computed(() => {
  if (!hasScannedImage.value) return null
  return `/images/scanned/${props.item.scanned_image_path}`
})

const conditions = [
  { value: 'M', label: 'Mint' },
  { value: 'NM', label: 'Near Mint' },
  { value: 'LP', label: 'Light Play' },
  { value: 'MP', label: 'Moderate Play' },
  { value: 'HP', label: 'Heavy Play' },
  { value: 'D', label: 'Damaged' }
]

const isPriceStale = computed(() => checkPriceStale(card.value))

const priceAge = computed(() => formatTimeAgo(card.value.price_updated_at))

const isPokemon = computed(() => card.value.game === 'pokemon')

const refreshPrice = async () => {
  if (!isPokemon.value) return

  refreshingPrice.value = true
  priceError.value = null

  try {
    const result = await priceService.refreshCardPrice(card.value.id)
    if (result.card) {
      // Update the card's price data
      card.value.price_usd = result.card.price_usd
      card.value.price_foil_usd = result.card.price_foil_usd
      card.value.price_updated_at = result.card.price_updated_at
      card.value.price_source = result.card.price_source
      emit('priceUpdated', result.card)
    }
  } catch (err) {
    if (err.response?.status === 429) {
      priceError.value = 'Daily quota exceeded'
    } else {
      priceError.value = 'Failed to refresh price'
    }
  } finally {
    refreshingPrice.value = false
  }
}

const handleAdd = () => {
  emit('add', {
    cardId: card.value.id,
    quantity: quantity.value,
    condition: condition.value,
    foil: foil.value,
    firstEdition: firstEdition.value
  })
}

const handleUpdate = () => {
  emit('update', {
    id: props.item.id,
    quantity: quantity.value,
    condition: condition.value,
    foil: foil.value,
    firstEdition: firstEdition.value
  })
}

const handleRemove = () => {
  if (confirm('Are you sure you want to remove this card from your collection?')) {
    emit('remove', props.item.id)
  }
}
</script>

<template>
  <div
    class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4"
    @click.self="emit('close')"
    role="dialog"
    aria-modal="true"
    :aria-labelledby="'card-title-' + card.id"
  >
    <div class="bg-white dark:bg-gray-800 rounded-lg max-w-2xl w-full max-h-[90vh] overflow-y-auto">
      <div class="flex flex-col md:flex-row">
        <div class="md:w-1/2 p-4">
          <!-- Image toggle for scanned images -->
          <div v-if="hasScannedImage" class="flex gap-2 mb-3">
            <button
              @click="showScannedImage = false"
              class="flex-1 py-2 px-3 text-sm rounded-lg transition"
              :class="!showScannedImage ? 'bg-blue-600 text-white' : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300'"
            >
              Official
            </button>
            <button
              @click="showScannedImage = true"
              class="flex-1 py-2 px-3 text-sm rounded-lg transition"
              :class="showScannedImage ? 'bg-blue-600 text-white' : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300'"
            >
              My Scan
            </button>
          </div>
          <img
            :src="showScannedImage && scannedImageUrl ? scannedImageUrl : (card.image_url_large || card.image_url)"
            :alt="card.name + ' card image'"
            class="w-full rounded-lg shadow"
          />
        </div>
        <div class="md:w-1/2 p-6">
          <div class="flex justify-between items-start mb-4">
            <div>
              <h2 :id="'card-title-' + card.id" class="text-2xl font-bold text-gray-800 dark:text-white">{{ card.name }}</h2>
              <p class="text-gray-500 dark:text-gray-400">{{ card.set_name }}</p>
            </div>
            <button
              @click="emit('close')"
              class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              aria-label="Close card details"
            >
              <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          <div class="space-y-3 mb-6">
            <div class="flex justify-between">
              <span class="text-gray-600 dark:text-gray-400">Set:</span>
              <span class="font-medium text-gray-800 dark:text-white">{{ card.set_name }} ({{ card.set_code }})</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-600 dark:text-gray-400">Number:</span>
              <span class="font-medium text-gray-800 dark:text-white">{{ card.card_number }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-600 dark:text-gray-400">Rarity:</span>
              <span class="font-medium capitalize text-gray-800 dark:text-white">{{ card.rarity }}</span>
            </div>
            <div class="flex justify-between items-center">
              <span class="text-gray-600 dark:text-gray-400">Price:</span>
              <div class="flex items-center gap-2">
                <span class="font-medium text-green-600 dark:text-green-400">{{ formatPrice(card.price_usd) }}</span>
                <span v-if="priceAge" class="text-xs" :class="isPriceStale ? 'text-orange-500' : 'text-gray-400'">
                  ({{ priceAge }})
                </span>
              </div>
            </div>
            <div v-if="card.price_foil_usd" class="flex justify-between">
              <span class="text-gray-600 dark:text-gray-400">Foil Price:</span>
              <span class="font-medium text-yellow-600 dark:text-yellow-400">{{ formatPrice(card.price_foil_usd) }}</span>
            </div>
            <div v-if="isPokemon" class="flex justify-between items-center">
              <span class="text-gray-600 dark:text-gray-400">Price Status:</span>
              <div class="flex items-center gap-2">
                <span class="text-xs px-2 py-1 rounded" :class="{
                  'bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-200': card.price_source === 'api',
                  'bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-200': card.price_source === 'cached',
                  'bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-200': card.price_source === 'pending',
                  'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300': !card.price_source
                }">
                  {{ card.price_source || 'unknown' }}
                </span>
                <button
                  @click="refreshPrice"
                  :disabled="refreshingPrice"
                  class="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300 text-sm disabled:opacity-50"
                >
                  <span v-if="refreshingPrice">Refreshing...</span>
                  <span v-else>Refresh</span>
                </button>
              </div>
            </div>
            <div v-if="priceError" class="text-red-500 text-sm">
              {{ priceError }}
            </div>
          </div>

          <div class="border-t dark:border-gray-700 pt-4 space-y-4">
            <div>
              <label :for="'quantity-' + card.id" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Quantity</label>
              <input
                :id="'quantity-' + card.id"
                v-model.number="quantity"
                type="number"
                min="1"
                class="w-full border dark:border-gray-600 rounded-lg px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                aria-describedby="quantity-help"
              />
            </div>
            <div>
              <label :for="'condition-' + card.id" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Condition</label>
              <select :id="'condition-' + card.id" v-model="condition" class="w-full border dark:border-gray-600 rounded-lg px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-white">
                <option v-for="c in conditions" :key="c.value" :value="c.value">
                  {{ c.label }}
                </option>
              </select>
            </div>
            <div class="flex items-center">
              <input
                v-model="foil"
                type="checkbox"
                :id="'foil-' + card.id"
                class="rounded border-gray-300 dark:border-gray-600 text-blue-600 mr-2"
              />
              <label :for="'foil-' + card.id" class="text-sm font-medium text-gray-700 dark:text-gray-300">Foil</label>
            </div>
            <div class="flex items-center">
              <input
                v-model="firstEdition"
                type="checkbox"
                :id="'first-edition-' + card.id"
                class="rounded border-gray-300 dark:border-gray-600 text-amber-600 mr-2"
              />
              <label :for="'first-edition-' + card.id" class="text-sm font-medium text-gray-700 dark:text-gray-300">1st Edition</label>
            </div>
          </div>

          <div class="mt-6 flex gap-3">
            <button
              v-if="!isCollectionItem"
              @click="handleAdd"
              class="flex-1 bg-blue-600 text-white py-2 px-4 rounded-lg hover:bg-blue-700 transition"
            >
              Add to Collection
            </button>
            <template v-else>
              <button
                @click="handleUpdate"
                class="flex-1 bg-blue-600 text-white py-2 px-4 rounded-lg hover:bg-blue-700 transition"
              >
                Update
              </button>
              <button
                @click="handleRemove"
                class="bg-red-600 text-white py-2 px-4 rounded-lg hover:bg-red-700 transition"
              >
                Remove
              </button>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
