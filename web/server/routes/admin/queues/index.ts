// Painel das filas (River UI), servido pela API em Go; só o admin entra.
export default defineEventHandler(event => proxyToApi(event, '/admin/queues'))
