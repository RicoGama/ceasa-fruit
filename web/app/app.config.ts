// Tema do Nuxt UI. "brand" e "base" são paletas definidas em assets/css/main.css,
// que mudam conforme o tema escolhido (azul ou verde) sem rebuild.
export default defineAppConfig({
  ui: {
    colors: {
      primary: 'brand',
      neutral: 'base',
    },
  },
})
