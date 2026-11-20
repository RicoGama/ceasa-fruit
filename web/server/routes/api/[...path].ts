// /api/qualquer/coisa -> API em Go: /qualquer/coisa
export default defineEventHandler(event =>
  proxyToApi(event, '/' + (getRouterParam(event, 'path') ?? '')),
)
