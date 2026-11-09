import type { City } from '~/types'

export interface CepAddress {
  street: string
  district: string
  city: City
}

// Consulta o CEP no ViaCEP (serviço gratuito) direto do navegador.
// A resposta já traz o código IBGE do município, igual ao da nossa tabela.
export async function lookupCep(cep: string): Promise<CepAddress | null> {
  const digits = cep.replace(/\D/g, '')
  if (digits.length !== 8) return null
  try {
    const r = await $fetch<{
      erro?: boolean | string
      logradouro: string
      bairro: string
      localidade: string
      uf: string
      ibge: string
    }>(`https://viacep.com.br/ws/${digits}/json/`, { timeout: 5000 })
    if (r.erro) return null
    return {
      street: r.logradouro,
      district: r.bairro,
      city: { id: Number(r.ibge), name: r.localidade, state: r.uf },
    }
  }
  catch {
    return null // sem internet ou serviço fora do ar: o usuário preenche à mão
  }
}
