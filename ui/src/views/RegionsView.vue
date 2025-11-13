<template>
  <div class="regions-view">
    <div class="page-header">
      <h2 class="page-title">Regions</h2>
      <wt-button
        @click="loadRegions"
        :disabled="loading"
        :loading="loading"
      >
        Refresh
      </wt-button>
    </div>

    <div v-if="error" class="error-message">
      {{ error }}
    </div>

    <div v-if="loading && regions.length === 0" class="loading-state">
      Loading regions...
    </div>

    <div v-else class="regions-grid">
      <div
        v-for="region in regions"
        :key="region.name"
        class="region-card"
      >
        <div class="region-card__header">
          <h3 class="region-card__name">{{ region.name }}</h3>
          <wt-indicator
            :color="getStatusColor(region.status)"
            :text="region.status"
          />
        </div>

        <div class="region-card__datacenters">
          <h4 class="region-card__subtitle">Datacenters</h4>
          <div class="datacenters-list">
            <div
              v-for="dc in region.datacenters"
              :key="dc.name"
              class="datacenter-item"
            >
              <div class="datacenter-item__header">
                <div
                  class="datacenter-item__info"
                  @click="goToDatacenter(dc.name)"
                >
                  <span class="datacenter-item__name">{{ dc.name }}</span>
                  <wt-indicator
                    :color="getStatusColor(dc.status)"
                    :text="dc.status"
                    size="sm"
                  />
                </div>
                <wt-switcher
                  :key="`${dc.name}-${switcherKey}`"
                  :model-value="datacenterEnabled[dc.name]"
                  @update:model-value="handleDatacenterToggle(dc.name, region.name, $event)"
                  :disabled="togglingDatacenter === dc.name || region.status !== 'active' || dc.status === 'error' || isLastEnabledInRegion(dc.name, region.name)"
                />
              </div>
              <div
                class="datacenter-item__stats"
                @click="goToDatacenter(dc.name)"
              >
                <span class="stat">
                  <span class="stat__label">Total:</span>
                  <span class="stat__value">{{ dc.nodes_total }}</span>
                </span>
                <span class="stat stat--success">
                  <span class="stat__label">Ready:</span>
                  <span class="stat__value">{{ dc.nodes_ready }}</span>
                </span>
                <span class="stat stat--warning">
                  <span class="stat__label">Draining:</span>
                  <span class="stat__value">{{ dc.nodes_draining }}</span>
                </span>
              </div>
            </div>
          </div>
        </div>

        <div class="region-card__actions">
          <wt-button
            @click="showActivateConfirm(region.name)"
            :disabled="activating === region.name || region.status === 'active' || region.status === 'error'"
            :loading="activating === region.name"
            wide
          >
            Activate Region
          </wt-button>
        </div>
      </div>
    </div>

    <!-- Activation confirmation popup -->
    <wt-popup
      v-if="showConfirmPopup"
      size="sm"
      @close="closeConfirmPopup"
    >
      <template #title>Confirm Activation</template>
      <template #main>
        <div class="confirm-content">
          <p>Are you sure you want to activate region <strong>{{ regionToActivate }}</strong>?</p>
          <p class="confirm-warning">This will drain all datacenters in other regions.</p>
        </div>
      </template>
      <template #actions>
        <wt-button
          color="secondary"
          @click="closeConfirmPopup"
        >
          Cancel
        </wt-button>
        <wt-button
          @click="confirmActivate"
          :loading="activating === regionToActivate"
        >
          Activate
        </wt-button>
      </template>
    </wt-popup>
  </div>
</template>

<script>
import { ref, onMounted, inject, watch } from 'vue'
import { useRouter } from 'vue-router'
import { regionsAPI, datacentersAPI } from '../api/client'

export default {
  name: 'RegionsView',
  setup() {
    const router = useRouter()
    const eventBus = inject('$eventBus')
    const regions = ref([])
    const loading = ref(false)
    const error = ref(null)
    const activating = ref(null)
    const showConfirmPopup = ref(false)
    const regionToActivate = ref(null)
    const datacenterEnabled = ref({})
    const togglingDatacenter = ref(null)
    const switcherKey = ref(0) // Force re-render key

    const getStatusColor = (status) => {
      const colorMap = {
        'active': 'success',
        'partial': 'info',
        'draining': 'error',
        'error': 'error',
      }
      return colorMap[status] || 'secondary'
    }

    const initializeDatacenterStates = () => {
      // Sync switcher states with actual datacenter status
      regions.value.forEach(region => {
        region.datacenters?.forEach(dc => {
          // Always sync with actual datacenter status
          datacenterEnabled.value[dc.name] = dc.status === 'active'
        })
      })
    }

    const loadRegions = async () => {
      loading.value = true
      error.value = null
      try {
        const response = await regionsAPI.getRegions()
        regions.value = response.data
        initializeDatacenterStates()
        // Force re-render of switchers to reflect actual state
        switcherKey.value++
      } catch (err) {
        error.value = err.response?.data?.error || err.message || 'Failed to load regions'
      } finally {
        loading.value = false
      }
    }

    const goToDatacenter = (datacenterName) => {
      router.push({ name: 'DatacenterDetail', params: { name: datacenterName } })
    }

    const isLastEnabledInRegion = (datacenterName, regionName) => {
      const region = regions.value.find(r => r.name === regionName)
      if (!region) return false

      // Count how many enabled datacenters in this region
      const enabledDCs = region.datacenters?.filter(
        dc => datacenterEnabled.value[dc.name]
      ) || []

      // If this DC is enabled and it's the only one enabled in region
      return datacenterEnabled.value[datacenterName] && enabledDCs.length === 1
    }

    const handleDatacenterToggle = async (datacenterName, regionName, enabled) => {
      togglingDatacenter.value = datacenterName
      error.value = null

      try {
        if (enabled) {
          // Enable: activate this datacenter
          await datacentersAPI.activateDatacenter(datacenterName)
          await loadRegions()

          eventBus.$emit('notification', {
            type: 'success',
            text: `Datacenter "${datacenterName}" activated successfully`,
            timeout: 4,
          })
        } else {
          // Disable: activate another datacenter in the region
          const region = regions.value.find(r => r.name === regionName)
          const otherEnabledDc = region?.datacenters?.find(
            dc => dc.name !== datacenterName && datacenterEnabled.value[dc.name]
          )

          if (otherEnabledDc) {
            await datacentersAPI.activateDatacenter(otherEnabledDc.name)
            await loadRegions()

            eventBus.$emit('notification', {
              type: 'success',
              text: `Datacenter "${datacenterName}" drained, "${otherEnabledDc.name}" activated`,
              timeout: 4,
            })
          }
          // Note: If no other enabled DC exists, this code won't be reached
          // because the switcher will be disabled by isLastEnabledInRegion check
        }
      } catch (err) {
        // Sync state with backend on error to revert switcher position
        await loadRegions()

        const errorMessage = err.response?.data?.error || err.message || 'Operation failed'
        error.value = errorMessage

        eventBus.$emit('notification', {
          type: 'error',
          text: `Failed to toggle datacenter "${datacenterName}": ${errorMessage}`,
          timeout: 6,
        })
      } finally {
        togglingDatacenter.value = null
      }
    }

    const showActivateConfirm = (regionName) => {
      regionToActivate.value = regionName
      showConfirmPopup.value = true
    }

    const closeConfirmPopup = () => {
      showConfirmPopup.value = false
      regionToActivate.value = null
    }

    const confirmActivate = async () => {
      const regionName = regionToActivate.value
      if (!regionName) return

      // Find enabled datacenters in this region
      const region = regions.value.find(r => r.name === regionName)
      const enabledDatacenters = region?.datacenters?.filter(
        dc => datacenterEnabled.value[dc.name]
      ) || []

      if (enabledDatacenters.length === 0) {
        // No enabled DCs - show warning but still drain all other regions
        eventBus.$emit('notification', {
          type: 'warning',
          text: `No enabled datacenters in region "${regionName}". All other regions will be drained. Please enable at least one datacenter.`,
          timeout: 6,
        })

        // Activate any datacenter in this region to drain others
        // User will need to manually enable DCs via switchers after
        if (region?.datacenters?.length > 0) {
          activating.value = regionName
          error.value = null
          try {
            await datacentersAPI.activateDatacenter(region.datacenters[0].name)
            await loadRegions()
            closeConfirmPopup()

            eventBus.$emit('notification', {
              type: 'info',
              text: `Region "${regionName}" prepared. Other regions drained. Please enable datacenters using switchers.`,
              timeout: 5,
            })
          } catch (err) {
            const errorMessage = err.response?.data?.error || err.message || 'Failed to activate region'
            error.value = errorMessage
            eventBus.$emit('notification', {
              type: 'error',
              text: `Failed to activate region "${regionName}": ${errorMessage}`,
              timeout: 6,
            })
          } finally {
            activating.value = null
          }
        }
        return
      }

      activating.value = regionName
      error.value = null

      try {
        // Activate each enabled datacenter separately
        for (let i = 0; i < enabledDatacenters.length; i++) {
          const dc = enabledDatacenters[i]
          await datacentersAPI.activateDatacenter(dc.name)

          // Small delay between activations to avoid race conditions
          if (i < enabledDatacenters.length - 1) {
            await new Promise(resolve => setTimeout(resolve, 500))
          }
        }

        await loadRegions()
        closeConfirmPopup()

        // Show success notification
        eventBus.$emit('notification', {
          type: 'success',
          text: `Region "${regionName}" activated with ${enabledDatacenters.length} datacenter(s)`,
          timeout: 4,
        })
      } catch (err) {
        const errorMessage = err.response?.data?.error || err.message || 'Failed to activate region'
        error.value = errorMessage

        // Show error notification
        eventBus.$emit('notification', {
          type: 'error',
          text: `Failed to activate region "${regionName}": ${errorMessage}`,
          timeout: 6,
        })
      } finally {
        activating.value = null
      }
    }

    onMounted(() => {
      loadRegions()
    })

    return {
      regions,
      loading,
      error,
      activating,
      showConfirmPopup,
      regionToActivate,
      datacenterEnabled,
      togglingDatacenter,
      switcherKey,
      loadRegions,
      goToDatacenter,
      isLastEnabledInRegion,
      handleDatacenterToggle,
      showActivateConfirm,
      closeConfirmPopup,
      confirmActivate,
      getStatusColor,
    }
  },
}
</script>

<style scoped>
.regions-view {
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  padding-bottom: 16px;
  border-bottom: 1px solid #e2e8f0;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #1a202c;
  margin: 0;
}

.error-message {
  padding: var(--spacing-sm, 12px) var(--spacing-md, 16px);
  background-color: var(--wt-color-error-light, #fee);
  border: 1px solid var(--wt-color-error, #fcc);
  border-radius: var(--border-radius, 4px);
  color: var(--wt-color-error-dark, #c00);
  margin-bottom: var(--spacing-md, 16px);
}

.loading-state {
  text-align: center;
  padding: var(--spacing-2xl, 48px);
  color: var(--wt-color-secondary, #718096);
}

.regions-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(400px, 1fr));
  gap: var(--spacing-lg, 24px);
}

.region-card {
  background: var(--wt-color-surface, #ffffff);
  border: 1px solid var(--wt-color-border, #e2e8f0);
  border-radius: var(--border-radius-lg, 8px);
  padding: var(--spacing-lg, 20px);
  transition: box-shadow 0.2s;
}

.region-card:hover {
  box-shadow: var(--box-shadow-md, 0 4px 6px rgba(0, 0, 0, 0.07));
}

.region-card__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md, 16px);
  padding-bottom: var(--spacing-md, 16px);
  border-bottom: 1px solid var(--wt-color-border, #e2e8f0);
}

.region-card__name {
  font-size: 18px;
  font-weight: 600;
  color: var(--wt-color-main, #1a202c);
}

.region-card__datacenters {
  margin-bottom: var(--spacing-md, 16px);
}

.region-card__subtitle {
  font-size: 14px;
  font-weight: 600;
  color: var(--wt-color-secondary, #4a5568);
  margin-bottom: var(--spacing-sm, 12px);
}

.datacenters-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm, 12px);
}

.datacenter-item {
  padding: var(--spacing-sm, 12px);
  background-color: var(--wt-color-background, #f7fafc);
  border-radius: var(--border-radius, 4px);
  border: 1px solid var(--wt-color-border, #e2e8f0);
  transition: border-color 0.2s;
}

.datacenter-item:hover {
  border-color: #cbd5e0;
}

.datacenter-item__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xs, 8px);
  gap: 12px;
}

.datacenter-item__info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex: 1;
  cursor: pointer;
  gap: 8px;
  padding: 4px;
  border-radius: 4px;
  transition: background-color 0.2s;
}

.datacenter-item__info:hover {
  background-color: rgba(0, 0, 0, 0.02);
}

.datacenter-item__name {
  font-weight: 500;
  color: var(--wt-color-main, #2d3748);
}

.datacenter-item__stats {
  display: flex;
  gap: var(--spacing-md, 16px);
  font-size: 13px;
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
  transition: background-color 0.2s;
}

.datacenter-item__stats:hover {
  background-color: rgba(0, 0, 0, 0.02);
}

.stat {
  display: flex;
  gap: var(--spacing-xs, 4px);
}

.stat__label {
  color: var(--wt-color-secondary, #718096);
}

.stat__value {
  font-weight: 600;
  color: var(--wt-color-main, #2d3748);
}

.stat--success .stat__value {
  color: var(--wt-color-success, #38a169);
}

.stat--warning .stat__value {
  color: var(--wt-color-warning, #dd6b20);
}

.region-card__actions {
  padding-top: var(--spacing-md, 16px);
  border-top: 1px solid var(--wt-color-border, #e2e8f0);
}

.confirm-content {
  padding: 16px 0;
  text-align: center;
}

.confirm-content p {
  margin: 0 0 12px 0;
  font-size: 15px;
  color: #1a202c;
}

.confirm-content p:last-child {
  margin-bottom: 0;
}

.confirm-warning {
  font-size: 13px;
  color: #64748b;
  font-style: italic;
}
</style>
