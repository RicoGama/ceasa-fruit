import { z } from 'zod'

// Mensagens de validação do Zod em português, para TODOS os formulários.
// Sem isto, um campo vazio mostraria "Invalid input: expected string, received
// undefined". Cada schema ainda pode passar a própria mensagem (ex.:
// z.string().min(8, 'A senha precisa...')), que tem prioridade sobre estas.
const isEmpty = (v: unknown) => v === undefined || v === null || v === ''

export default defineNuxtPlugin(() => {
  z.config({
    customError: (issue) => {
      switch (issue.code) {
        case 'invalid_type':
          if (isEmpty(issue.input)) return 'Campo obrigatório'
          return issue.expected === 'number' ? 'Informe um número' : 'Valor inválido'
        case 'too_small':
          if (issue.origin === 'string') {
            return Number(issue.minimum) <= 1 ? 'Campo obrigatório' : `Use pelo menos ${issue.minimum} caracteres`
          }
          if (issue.origin === 'array') return `Adicione pelo menos ${issue.minimum} item(ns)`
          return `O valor mínimo é ${issue.minimum}`
        case 'too_big':
          if (issue.origin === 'string') return `Use no máximo ${issue.maximum} caracteres`
          if (issue.origin === 'array') return `No máximo ${issue.maximum} item(ns)`
          return `O valor máximo é ${issue.maximum}`
        case 'invalid_format':
          if (issue.format === 'email') return 'E-mail inválido'
          if (issue.format === 'url') return 'Endereço inválido'
          return 'Formato inválido'
        case 'invalid_value':
          return 'Escolha uma das opções'
        default:
          return 'Valor inválido'
      }
    },
  })
})
