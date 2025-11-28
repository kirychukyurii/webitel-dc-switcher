<template>
  <div class="status-bar">
    <div class="status-item">
      <span class="label">Opened DC:</span>
      <span class="value">{{ status.my_datacenter || '-' }}</span>
    </div>
    <div class="status-item">
      <span class="label">Active:</span>
      <span
        class="value"
        :class="{ 'is-active': status.active_datacenter === status.my_datacenter }"
      >
        {{ status.active_datacenter || '-' }}
      </span>
    </div>
    <div class="status-item">
      <span class="label">Last seen:</span>
      <span
        class="value"
        :class="heartbeatClass"
      >
        {{ heartbeatText }}
      </span>
    </div>
    <div class="status-item">
      <span class="label">Status:</span>
      <span
        class="value status-badge"
        :class="statusClass"
      >
        {{ statusText }}
      </span>
    </div>
    <div class="status-item">
      <span class="label">etcd:</span>
      <span
        class="value"
        :class="{ 'connected': status.etcd_connected, 'disconnected': !status.etcd_connected }"
      >
        {{ status.etcd_connected ? 'connected' : 'disconnected' }}
      </span>
    </div>
  </div>
</template>

<script>
import { statusAPI } from '@/api/client'

export default {
  name: 'StatusBar',
  data() {
    return {
      status: {},
      refreshInterval: null,
    }
  },
  computed: {
    heartbeatText() {
      if (!this.status.heartbeat_age) return '-'
      const ageSeconds = Math.floor(this.status.heartbeat_age / 1000)
      if (ageSeconds < 60) return `${ageSeconds}s`
      return `${Math.floor(ageSeconds / 60)}m ${ageSeconds % 60}s`
    },
    heartbeatClass() {
      if (!this.status.heartbeat_age || !this.status.stale_threshold) return ''
      const isStale = this.status.heartbeat_age > this.status.stale_threshold
      return isStale ? 'stale' : 'fresh'
    },
    statusText() {
      if (!this.status.am_drained) return 'active'
      return 'drained'
    },
    statusClass() {
      if (!this.status.am_drained) return 'status-active'
      return 'status-drained'
    },
  },
  mounted() {
    this.fetchStatus()
    // Refresh every 5 seconds
    this.refreshInterval = setInterval(() => {
      this.fetchStatus()
    }, 5000)
  },
  beforeUnmount() {
    if (this.refreshInterval) {
      clearInterval(this.refreshInterval)
    }
  },
  methods: {
    async fetchStatus() {
      try {
        const response = await statusAPI.getStatus()
        this.status = response.data
      } catch (error) {
        console.error('Failed to fetch status:', error)
      }
    },
  },
}
</script>

<style scoped>
.status-bar {
  display: flex;
  gap: 24px;
  padding: 12px 16px;
  background-color: #f5f5f5;
  border-bottom: 1px solid #e0e0e0;
  font-size: 14px;
}

.status-item {
  display: flex;
  gap: 6px;
  align-items: center;
}

.label {
  font-weight: 500;
  color: #666;
}

.value {
  color: #333;
  font-weight: 600;
}

.value.is-active {
  color: #4caf50;
}

.value.fresh {
  color: #4caf50;
}

.value.stale {
  color: #ff9800;
}

.value.connected {
  color: #4caf50;
}

.value.disconnected {
  color: #f44336;
}

.status-badge {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  text-transform: uppercase;
}

.status-active {
  background-color: #e8f5e9;
  color: #2e7d32;
}

.status-drained {
  background-color: #fff3e0;
  color: #ef6c00;
}
</style>
