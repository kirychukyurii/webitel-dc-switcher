import axios from 'axios'

// Get base path from global variable injected by backend
const basePath = window._BASE_PATH || ''
const apiBaseURL = `${basePath}/api`

const apiClient = axios.create({
  baseURL: apiBaseURL,
  headers: {
    'Content-Type': 'application/json',
  },
})

export const regionsAPI = {
  // Get all regions
  getRegions() {
    return apiClient.get('/regions')
  },

  // Get datacenters by region
  getDatacentersByRegion(region) {
    return apiClient.get(`/regions/${region}/datacenters`)
  },

  // Activate region
  activateRegion(region) {
    return apiClient.post(`/regions/${region}/activate`)
  },
}

export const datacentersAPI = {
  // Get all datacenters
  getDatacenters() {
    return apiClient.get('/datacenters')
  },

  // Get nodes for datacenter
  getNodes(datacenter) {
    return apiClient.get(`/datacenters/${datacenter}/nodes`)
  },

  // Activate datacenter (drain all others)
  activateDatacenter(datacenter) {
    return apiClient.post(`/datacenters/${datacenter}/activate`)
  },

  // Get jobs for datacenter
  getJobs(datacenter) {
    return apiClient.get(`/datacenters/${datacenter}/jobs`)
  },

  // Start a job
  startJob(datacenter, jobId) {
    return apiClient.post(`/datacenters/${datacenter}/jobs/${jobId}/start`)
  },

  // Stop a job
  stopJob(datacenter, jobId) {
    return apiClient.post(`/datacenters/${datacenter}/jobs/${jobId}/stop`)
  },
}

export const statusAPI = {
  // Get service status including heartbeat info
  getStatus() {
    return apiClient.get('/status')
  },
}

export default apiClient
