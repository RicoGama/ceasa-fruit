import { z } from 'zod'
import type { City, PartyMatch, SupplierFields } from '~/types'

// O que fornecedor e cliente têm em comum: a PESSOA (identificação, contato,
// endereço, pagamento, fiscal). Na API é o pacote party; aqui é este arquivo
// + os componentes em components/party.
//
// Cada formulário (SupplierForm, CustomerForm) junta este estado com os
// campos do seu papel: { ...emptyPartyState(), supplierType: ..., status: ... }.

// --- Estado da tela -------------------------------------------------------------
// Campos de texto usam '' (vazio) na tela; na hora de enviar viram undefined.

export interface ContactRow { name: string, role: string, phone: string, isWhatsapp: boolean, email: string }

type Payment = NonNullable<SupplierFields['payment']>

export function emptyPartyState() {
  return {
    personType: 'company' as 'company' | 'individual',
    document: '',
    legalName: '',
    tradeName: '',
    stateRegistration: '',
    taxpayerType: 'non_taxpayer' as SupplierFields['taxpayerType'],
    email: '',
    phone: '',
    whatsapp: '',
    address: { postalCode: '', street: '', number: '', complement: '', district: '', city: null as City | null },
    payment: {
      pixKeyType: undefined as Payment['pixKeyType'],
      pixKey: '',
      bank: '',
      bankBranch: '',
      bankAccount: '',
      bankAccountType: undefined as Payment['bankAccountType'],
      bankAccountHolder: '',
      bankAccountHolderDocument: '',
    },
    contacts: [] as ContactRow[],
    notes: '',
  }
}
export type PartyState = ReturnType<typeof emptyPartyState>

// A pessoa como a API devolve (Supplier, Customer e PartyMatch têm estes campos).
export type PartyData = Pick<PartyMatch,
  'personType' | 'document' | 'legalName' | 'tradeName' | 'stateRegistration' | 'taxpayerType'
  | 'email' | 'phone' | 'whatsapp' | 'address' | 'payment' | 'contacts' | 'notes'>

// Converte a pessoa que veio da API para o estado da tela.
export function partyStateFrom(p: PartyData): PartyState {
  const v = (x?: string | null) => x ?? ''
  return {
    personType: p.personType,
    document: p.document ? formatDocument(p.document) : '',
    legalName: p.legalName,
    tradeName: v(p.tradeName),
    stateRegistration: v(p.stateRegistration),
    taxpayerType: p.taxpayerType,
    email: v(p.email),
    phone: formatPhone(p.phone),
    whatsapp: formatPhone(p.whatsapp),
    address: {
      postalCode: formatPostalCode(p.address.postalCode),
      street: v(p.address.street),
      number: v(p.address.number),
      complement: v(p.address.complement),
      district: v(p.address.district),
      city: p.address.city,
    },
    payment: {
      pixKeyType: p.payment.pixKeyType ?? undefined,
      pixKey: v(p.payment.pixKey),
      bank: v(p.payment.bank),
      bankBranch: v(p.payment.bankBranch),
      bankAccount: v(p.payment.bankAccount),
      bankAccountType: p.payment.bankAccountType ?? undefined,
      bankAccountHolder: v(p.payment.bankAccountHolder),
      bankAccountHolderDocument: p.payment.bankAccountHolderDocument ? formatDocument(p.payment.bankAccountHolderDocument) : '',
    },
    contacts: (p.contacts ?? []).map(c => ({
      name: c.name, role: v(c.role), phone: formatPhone(c.phone), isWhatsapp: c.isWhatsapp, email: v(c.email),
    })),
    notes: v(p.notes),
  }
}

// --- Envio ----------------------------------------------------------------------

// '' vira undefined: a API trata campo ausente como "não informado" (NULL).
export const blankToUndefined = (s?: string | null) => (s?.trim() ? s.trim() : undefined)

// Monta a parte "pessoa" do corpo enviado à API.
export function partyBody(v: PartyState) {
  const t = blankToUndefined
  return {
    personType: v.personType,
    document: t(v.document),
    legalName: v.legalName.trim(),
    tradeName: t(v.tradeName),
    stateRegistration: t(v.stateRegistration),
    taxpayerType: v.taxpayerType,
    email: t(v.email),
    phone: t(v.phone),
    whatsapp: t(v.whatsapp),
    address: {
      postalCode: t(v.address.postalCode),
      street: t(v.address.street),
      number: t(v.address.number),
      complement: t(v.address.complement),
      district: t(v.address.district),
      cityId: v.address.city?.id,
    },
    payment: {
      pixKeyType: v.payment.pixKeyType || undefined,
      pixKey: t(v.payment.pixKey),
      bank: t(v.payment.bank),
      bankBranch: t(v.payment.bankBranch),
      bankAccount: t(v.payment.bankAccount),
      bankAccountType: v.payment.bankAccountType || undefined,
      bankAccountHolder: t(v.payment.bankAccountHolder),
      bankAccountHolderDocument: t(v.payment.bankAccountHolderDocument),
    },
    contacts: v.contacts.map(c => ({
      name: c.name.trim(), role: t(c.role), phone: t(c.phone), isWhatsapp: c.isWhatsapp, email: t(c.email),
    })),
    notes: t(v.notes),
  }
}

// --- Validação (mesmas regras da API, mensagens em português) ---------------

export const textField = (max: number) => z.string().max(max, `Máximo de ${max} caracteres`)
const phoneField = textField(20).refine(v => !v || (onlyDigits(v).length >= 8 && onlyDigits(v).length <= 13), 'Telefone inválido')
const emailField = z.union([z.literal(''), z.email('E-mail inválido')])

// Campos da pessoa; cada formulário faz .extend({...}) com os do seu papel.
export const partySchema = z.object({
  personType: z.enum(['company', 'individual']),
  document: z.string().refine(v => !v || isValidDocument(v), 'CPF ou CNPJ inválido'),
  legalName: z.string().trim().min(2, 'Informe pelo menos 2 caracteres').max(150, 'Máximo de 150 caracteres'),
  tradeName: textField(150),
  stateRegistration: textField(20),
  taxpayerType: z.string(),
  email: emailField,
  phone: phoneField,
  whatsapp: phoneField,
  address: z.object({
    postalCode: z.string().refine(v => !v || onlyDigits(v).length === 8, 'CEP inválido'),
    street: textField(150),
    number: textField(20),
    complement: textField(100),
    district: textField(100),
    city: z.any(),
  }),
  payment: z.object({
    pixKeyType: z.string().optional(),
    pixKey: textField(100),
    bank: textField(100),
    bankBranch: textField(20),
    bankAccount: textField(30),
    bankAccountType: z.string().optional(),
    bankAccountHolder: textField(150),
    bankAccountHolderDocument: z.string().refine(v => !v || isValidDocument(v), 'CPF ou CNPJ inválido'),
  }),
  contacts: z.array(z.object({
    name: z.string().trim().min(1, 'Informe o nome').max(100),
    role: textField(60),
    phone: phoneField,
    isWhatsapp: z.boolean(),
    email: emailField,
  })).max(20, 'Máximo de 20 contatos'),
  notes: textField(2000),
})

// Regras que dependem de mais de um campo da pessoa. Use dentro do
// .superRefine do formulário: checkPartyRules(v, ctx).
export function checkPartyRules(v: z.output<typeof partySchema>, ctx: z.RefinementCtx) {
  const doc = normalizeDocument(v.document)
  if (doc && v.personType === 'company' && doc.length !== 14) {
    ctx.addIssue({ code: 'custom', path: ['document'], message: 'Pessoa jurídica precisa de CNPJ' })
  }
  if (doc && v.personType === 'individual' && doc.length !== 11) {
    ctx.addIssue({ code: 'custom', path: ['document'], message: 'Pessoa física precisa de CPF' })
  }
  if (v.payment.pixKeyType && !v.payment.pixKey.trim()) {
    ctx.addIssue({ code: 'custom', path: ['payment', 'pixKey'], message: 'Informe a chave PIX' })
  }
  if (!v.payment.pixKeyType && v.payment.pixKey.trim()) {
    ctx.addIssue({ code: 'custom', path: ['payment', 'pixKeyType'], message: 'Informe o tipo da chave' })
  }
}
