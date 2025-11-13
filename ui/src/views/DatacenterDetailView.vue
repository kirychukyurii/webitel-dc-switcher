<template>
  <div class="datacenter-detail">
    <div class="page-header">
      <div class="header-left">
        <wt-button
          color="secondary"
          @click="$router.back()"
        >
          ← Back
        </wt-button>
        <h2 class="page-title">{{ name }}</h2>
        <wt-indicator
          :color="getStatusColor(datacenter?.status)"
          :text="datacenter?.status || 'loading'"
        />
      </div>
      <wt-button
        @click="loadNodes"
        :disabled="loading"
        :loading="loading"
      >
        Refresh
      </wt-button>
    </div>

    <div v-if="error" class="error-message">
      {{ error }}
    </div>

    <div v-if="datacenter" class="datacenter-stats">
      <div class="stat-card">
        <div class="stat-card__value">{{ datacenter.nodes_total }}</div>
        <div class="stat-card__label">Total Nodes</div>
      </div>
      <div class="stat-card stat-card--success">
        <div class="stat-card__value">{{ datacenter.nodes_ready }}</div>
        <div class="stat-card__label">Ready</div>
      </div>
      <div class="stat-card stat-card--warning">
        <div class="stat-card__value">{{ datacenter.nodes_draining }}</div>
        <div class="stat-card__label">Draining</div>
      </div>
    </div>

    <div v-if="loading && nodes.length === 0" class="loading-state">
      Loading nodes...
    </div>

    <div v-else-if="nodes.length > 0" class="nodes-section">
      <h3 class="section-title">Nodes</h3>
      <wt-table
        :headers="nodeHeaders"
        :data="nodesTableData"
        class="nodes-table"
        sortable
        resizable-columns
        @sort="sort"
      >
        <template #status="{ item }">
          <wt-indicator
            :color="getNodeStatusColor(item.status)"
            :text="item.status"
            size="md"
          />
        </template>
        <template #drain="{ item }">
          <wt-indicator
            :color="item.drain ? 'error' : 'success'"
            :text="item.drain ? 'Yes' : 'No'"
            size="md"
          />
        </template>
        <template #scheduling_eligibility="{ item }">
          <wt-indicator
            :color="item.scheduling_eligibility === 'eligible' ? 'success' : 'secondary'"
            :text="item.scheduling_eligibility"
            size="md"
          />
        </template>
      </wt-table>
    </div>

    <div v-else class="empty-state">
      No nodes found
    </div>
  </div>
</template>

<script>
import { ref, onMounted, computed } from 'vue'
import { datacentersAPI } from '../api/client'

export default {
  name: 'DatacenterDetailView',
  props: {
    name: {
      type: String,
      required: true,
    },
  },
  setup(props) {
    const nodes = ref([])
    const datacenter = ref(null)
    const loading = ref(false)
    const error = ref(null)

    const nodeHeaders = ref([
      { text: 'Name', value: 'name', sort: null },
      { text: 'ID', value: 'id', sort: null },
      { text: 'Status', value: 'status', sort: null },
      { text: 'Drain', value: 'drain', sort: null },
      { text: 'Eligibility', value: 'scheduling_eligibility', sort: null },
    ])

    const nodesTableData = computed(() => {
      return nodes.value
    })

    const getStatusColor = (status) => {
      const colorMap = {
        'active': 'success',
        'draining': 'error',
        'error': 'error',
      }
      return colorMap[status] || 'secondary'
    }

    const getNodeStatusColor = (status) => {
      const colorMap = {
        'ready': 'success',
        'down': 'error',
        'initializing': 'info',
      }
      return colorMap[status] || 'secondary'
    }

    const loadDatacenterInfo = async () => {
      try {
        const response = await datacentersAPI.getDatacenters()
        const dc = response.data.find(d => d.name === props.name)
        if (dc) {
          datacenter.value = dc
        }
      } catch (err) {
        console.error('Failed to load datacenter info:', err)
      }
    }

    const loadNodes = async () => {
      loading.value = true
      error.value = null
      try {
        const response = await datacentersAPI.getNodes(props.name)
        nodes.value = response.data
        await loadDatacenterInfo()
      } catch (err) {
        error.value = err.response?.data?.error || err.message || 'Failed to load nodes'
      } finally {
        loading.value = false
      }
    }

    const sort = (col, sortValue) => {
      // Reset all other columns' sort
      nodeHeaders.value.forEach((header) => {
        header.sort = null
      })
      // Set the current column's sort
      const column = nodeHeaders.value.find((header) => header.value === col.value)
      if (column) {
        column.sort = sortValue
      }
    }

    onMounted(() => {
      loadNodes()
    })

    return {
      nodes,
      datacenter,
      loading,
      error,
      nodeHeaders,
      nodesTableData,
      loadNodes,
      getStatusColor,
      getNodeStatusColor,
      sort,
    }
  },
}
</script>

<style scoped>
.datacenter-detail {
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

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #1a202c;
  margin: 0;
}

.error-message {
  padding: 12px 16px;
  background-color: #fee;
  border: 1px solid #fcc;
  border-radius: 4px;
  color: #c00;
  margin-bottom: 16px;
}

.loading-state {
  text-align: center;
  padding: 48px;
  color: #718096;
}

.empty-state {
  text-align: center;
  padding: 48px;
  color: #718096;
  font-size: 16px;
}

.datacenter-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-bottom: 32px;
}

.stat-card {
  padding: 20px;
  background-color: #f7fafc;
  border-radius: 8px;
  text-align: center;
  border: 1px solid #e2e8f0;
}

.stat-card__value {
  font-size: 32px;
  font-weight: 700;
  color: #2d3748;
  margin-bottom: 8px;
}

.stat-card--success .stat-card__value {
  color: #38a169;
}

.stat-card--warning .stat-card__value {
  color: #dd6b20;
}

.stat-card__label {
  font-size: 13px;
  color: #718096;
  text-transform: uppercase;
  font-weight: 500;
  letter-spacing: 0.5px;
}

.nodes-section {
  margin-top: 24px;
}

.section-title {
  font-size: 18px;
  font-weight: 600;
  color: #1a202c;
  margin-bottom: 16px;
}

.nodes-table {
  background-color: #ffffff;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  overflow: hidden;
}
</style>
