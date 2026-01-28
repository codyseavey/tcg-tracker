import { defineStore } from 'pinia'
import { bulkImportService } from '../services/api'

const STORAGE_KEY = 'tcg-bulk-import-job'
const STORAGE_KEY_STATE = 'tcg-bulk-import-state'

// Helper to load persisted state
function loadPersistedState() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY_STATE)
    if (stored) {
      return JSON.parse(stored)
    }
  } catch {
    // Ignore parse errors
  }
  return null
}

// Helper to save state to localStorage
function persistState(state) {
  try {
    localStorage.setItem(STORAGE_KEY_STATE, JSON.stringify({
      phase: state.phase,
      progress: state.job?.processed_items || 0,
      total: state.job?.total_items || 0,
      lastPollTime: Date.now(),
      jobStatus: state.job?.status || null
    }))
  } catch {
    // Ignore storage errors
  }
}

export const useBulkImportStore = defineStore('bulkImport', {
  state: () => {
    const persisted = loadPersistedState()
    return {
      // Current job data
      job: null,
      jobId: localStorage.getItem(STORAGE_KEY) || null,
      
      // UI state
      phase: persisted?.phase || 'upload', // 'upload', 'processing', 'review'
      loading: false,
      uploading: false,
      uploadProgress: 0,
      error: null,
      
      // Resume state - show last known progress while fetching fresh data
      resuming: false,
      lastKnownProgress: persisted?.progress || 0,
      lastKnownTotal: persisted?.total || 0,
      
      // Polling
      pollInterval: null,
      
      // Search for manual card selection (grouped by set)
      searchResults: [], // Array of set groups: { set_code, set_name, cards: [...] }
      searchTotalCards: 0,
      searchLoading: false
    }
  },

  getters: {
    // Count of items by status
    pendingCount: (state) => state.job?.items?.filter(i => i.status === 'pending').length || 0,
    processingCount: (state) => state.job?.items?.filter(i => i.status === 'processing').length || 0,
    identifiedCount: (state) => state.job?.items?.filter(i => i.status === 'identified').length || 0,
    failedCount: (state) => state.job?.items?.filter(i => i.status === 'failed').length || 0,
    confirmedCount: (state) => state.job?.items?.filter(i => i.status === 'confirmed').length || 0,
    
    // Items grouped by status for display
    identifiedItems: (state) => state.job?.items?.filter(i => i.status === 'identified') || [],
    failedItems: (state) => state.job?.items?.filter(i => i.status === 'failed') || [],
    processingItems: (state) => state.job?.items?.filter(i => i.status === 'processing' || i.status === 'pending') || [],
    
    // Progress percentage - use last known progress while resuming
    progress: (state) => {
      if (state.resuming && state.lastKnownTotal > 0) {
        return Math.round((state.lastKnownProgress / state.lastKnownTotal) * 100)
      }
      if (!state.job || !state.job.total_items) return 0
      return Math.round((state.job.processed_items / state.job.total_items) * 100)
    },
    
    // Display progress values (use last known while resuming)
    displayProcessed: (state) => {
      if (state.resuming && !state.job) return state.lastKnownProgress
      return state.job?.processed_items || 0
    },
    displayTotal: (state) => {
      if (state.resuming && !state.job) return state.lastKnownTotal
      return state.job?.total_items || 0
    },
    
    // Whether the job is complete (all items processed)
    isComplete: (state) => {
      if (!state.job) return false
      return state.job.status === 'completed'
    },
    
    // Whether we have a job (active or recent)
    hasJob: (state) => !!state.job || state.resuming,
    
    // Items that can be confirmed (identified and not skipped)
    confirmableItems: (state) => state.job?.items?.filter(i => i.status === 'identified') || [],
    confirmableCount: (state) => state.job?.items?.filter(i => i.status === 'identified').length || 0
  },

  actions: {
    clearError() {
      this.error = null
    },
    
    /**
     * Initialize the store - check for existing job
     * Handles page refresh during processing by showing last known state
     */
    async init() {
      if (this.jobId) {
        // If we have a stored job ID and were in processing phase, show resuming state
        const persisted = loadPersistedState()
        if (persisted?.phase === 'processing' && persisted?.jobStatus !== 'completed') {
          this.resuming = true
          this.phase = 'processing'
          this.lastKnownProgress = persisted.progress || 0
          this.lastKnownTotal = persisted.total || 0
          // Start polling immediately while we fetch
          this.startPolling()
        }
        
        await this.fetchJob()
        this.resuming = false
      }
    },
    
    /**
     * Create a new bulk import job with uploaded files
     * @param {FileList|File[]} files - Image files to upload
     */
    async createJob(files) {
      this.uploading = true
      this.uploadProgress = 0
      this.error = null
      
      try {
        const result = await bulkImportService.createJob(files, (loaded, total) => {
          this.uploadProgress = Math.round((loaded / total) * 100)
        })
        
        // Store job ID for persistence
        this.jobId = result.job_id
        localStorage.setItem(STORAGE_KEY, result.job_id)
        
        // Fetch full job data
        await this.fetchJob()
        
        // Start polling for updates
        this.startPolling()
        
        // Move to processing phase
        this.phase = 'processing'
        
        // Persist state for resume after refresh
        persistState(this)
        
        return result
      } catch (err) {
        this.error = err.message
        throw err
      } finally {
        this.uploading = false
      }
    },
    
    /**
     * Fetch the current job data
     */
    async fetchJob() {
      if (!this.jobId) {
        // Try to get current job from server
        try {
          this.job = await bulkImportService.getCurrentJob()
          this.jobId = this.job.id
          localStorage.setItem(STORAGE_KEY, this.job.id)
          this.updatePhase()
          persistState(this)
        } catch {
          // No job found, that's ok
          this.job = null
          this.phase = 'upload'
          this.resuming = false
        }
        return
      }
      
      this.loading = true
      try {
        this.job = await bulkImportService.getJob(this.jobId)
        this.resuming = false // We have fresh data now
        this.updatePhase()
        persistState(this) // Save progress to localStorage
      } catch (err) {
        // Job not found, clear it
        if (err.response?.status === 404) {
          this.clearJob()
        } else {
          this.error = err.message
        }
        this.resuming = false
      } finally {
        this.loading = false
      }
    },
    
    /**
     * Update phase based on job status
     */
    updatePhase() {
      if (!this.job) {
        this.phase = 'upload'
        return
      }
      
      if (this.job.status === 'completed') {
        this.phase = 'review'
        this.stopPolling()
      } else if (this.job.status === 'processing' || this.job.status === 'pending') {
        this.phase = 'processing'
      }
    },
    
    /**
     * Start polling for job updates
     */
    startPolling() {
      this.stopPolling()
      this.pollInterval = setInterval(async () => {
        await this.fetchJob()
        if (this.isComplete) {
          this.stopPolling()
        }
      }, 3000) // Poll every 3 seconds
    },
    
    /**
     * Stop polling for job updates
     */
    stopPolling() {
      if (this.pollInterval) {
        clearInterval(this.pollInterval)
        this.pollInterval = null
      }
    },
    
    /**
     * Update an item's card selection or attributes
     * @param {number} itemId - Item ID
     * @param {Object} updates - { card_id, condition, printing_type, language }
     */
    async updateItem(itemId, updates) {
      if (!this.jobId) return
      
      this.loading = true
      try {
        const updatedItem = await bulkImportService.updateItem(this.jobId, itemId, updates)
        
        // Update item in local state
        const index = this.job.items.findIndex(i => i.id === itemId)
        if (index !== -1) {
          this.job.items[index] = updatedItem
        }
        
        return updatedItem
      } catch (err) {
        this.error = err.message
        throw err
      } finally {
        this.loading = false
      }
    },
    
    /**
     * Confirm and add items to collection
     * @param {number[]} [itemIds] - Specific items to confirm, or null for all
     */
    async confirmItems(itemIds = null) {
      if (!this.jobId) return
      
      this.loading = true
      try {
        const result = await bulkImportService.confirmJob(this.jobId, itemIds)
        
        // Refresh job to get updated item statuses
        await this.fetchJob()
        
        return result
      } catch (err) {
        this.error = err.message
        throw err
      } finally {
        this.loading = false
      }
    },
    
    /**
     * Delete the current job and start fresh
     */
    async deleteJob() {
      if (!this.jobId) return
      
      this.loading = true
      try {
        await bulkImportService.deleteJob(this.jobId)
        this.clearJob()
      } catch (err) {
        this.error = err.message
        throw err
      } finally {
        this.loading = false
      }
    },
    
    /**
     * Clear job from state and storage
     */
    clearJob() {
      this.stopPolling()
      this.job = null
      this.jobId = null
      this.phase = 'upload'
      this.resuming = false
      this.lastKnownProgress = 0
      this.lastKnownTotal = 0
      localStorage.removeItem(STORAGE_KEY)
      localStorage.removeItem(STORAGE_KEY_STATE)
    },
    
    /**
     * Search for cards (for manual selection)
     * @param {string} query - Search query
     * @param {string} game - 'pokemon' or 'mtg'
     */
    async searchCards(query, game = 'pokemon') {
      this.searchLoading = true
      try {
        const result = await bulkImportService.searchCards(query, game)
        // Results are now grouped by set: { set_groups: [...], total_cards: N }
        this.searchResults = result.set_groups || []
        this.searchTotalCards = result.total_cards || 0
        return result
      } catch (err) {
        this.error = err.message
        return { set_groups: [], total_cards: 0 }
      } finally {
        this.searchLoading = false
      }
    },
    
    /**
     * Clear search results
     */
    clearSearch() {
      this.searchResults = []
      this.searchTotalCards = 0
    }
  }
})
