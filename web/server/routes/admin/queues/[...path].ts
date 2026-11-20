export default defineEventHandler(event =>
  proxyToApi(event, '/admin/queues/' + (getRouterParam(event, 'path') ?? '')),
)
