package auth

import (
	"slices"

	"github.com/danielgtaylor/huma/v2"
)

// Permission é uma ação que um papel pode ou não fazer.
//
// REGRA: o catálogo de permissões fica AQUI, no código, e não no banco. Uma
// permissão só tem efeito se alguma rota a conferir (RequirePermission); se o
// usuário pudesse inventar permissões pela tela, elas não fariam nada.
// Os PAPÉIS (quais permissões cada um tem) é que ficam no banco e são
// editados pelo administrador. Os nomes em português ficam no front.
//
// Ao criar um módulo novo: acrescente as permissões aqui (e em All), use-as
// nas rotas e coloque o texto em português em web/app/utils/permissions.ts.
type Permission string

const (
	SuppliersView Permission = "suppliers.view" // ver fornecedores
	SuppliersEdit Permission = "suppliers.edit" // cadastrar, editar e inativar fornecedores
	CustomersView Permission = "customers.view" // ver clientes
	CustomersEdit Permission = "customers.edit" // cadastrar, editar e inativar clientes
	FleetView     Permission = "fleet.view"     // ver o mapa da frota, veículos e motoristas
	FleetEdit     Permission = "fleet.edit"     // cadastrar veículos e motoristas
	ProductsView  Permission = "products.view"  // ver produtos
	ProductsEdit  Permission = "products.edit"  // cadastrar, editar e inativar produtos
	UsersManage   Permission = "users.manage"   // usuários, papéis e permissões
	CompanyEdit   Permission = "company.edit"   // alterar os dados da empresa
	QueuesView    Permission = "queues.view"    // painel da fila de tarefas
	AuditView     Permission = "audit.view"     // registro de acessos (quem entrou, quando, de onde)
)

// All é o catálogo completo, na ordem em que aparece na tela.
var All = []Permission{
	SuppliersView, SuppliersEdit,
	CustomersView, CustomersEdit,
	ProductsView, ProductsEdit,
	FleetView, FleetEdit,
	UsersManage, CompanyEdit, QueuesView, AuditView,
}

// Implies: quem tem a permissão da esquerda também precisa da da direita
// (não faz sentido editar fornecedor sem poder vê-lo).
var Implies = map[Permission]Permission{
	SuppliersEdit: SuppliersView,
	CustomersEdit: CustomersView,
	ProductsEdit:  ProductsView,
	FleetEdit:     FleetView,
}

// Valid diz se a permissão existe no catálogo.
func (p Permission) Valid() bool { return slices.Contains(All, p) }

// Schema faz o OpenAPI listar as permissões válidas (o Huma recusa as
// desconhecidas, e o front recebe o tipo com todas elas).
func (Permission) Schema(huma.Registry) *huma.Schema {
	enum := make([]any, len(All))
	for i, p := range All {
		enum[i] = string(p)
	}
	return &huma.Schema{Type: huma.TypeString, Enum: enum}
}

// RoleKeyAdmin é a chave do papel de sistema "Administrador": tem todas as
// permissões e não pode ser alterado nem apagado.
const RoleKeyAdmin = "admin"
