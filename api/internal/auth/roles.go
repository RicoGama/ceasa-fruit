package auth

// Papéis (perfis de acesso) dos usuários. Os valores são gravados no banco
// (users.role) e aparecem na API; os nomes em português ficam no front.
const (
	RoleAdmin   = "admin"   // tudo, inclusive usuários e configurações
	RoleManager = "manager" // gerente
	RoleSeller  = "seller"  // vendedor
	RoleChecker = "checker" // conferente: entrada e saída de mercadoria
	RoleFinance = "finance" // financeiro
	RoleDriver  = "driver"  // motorista
)

// RoleEnum é a lista para as tags de validação do Huma (enum:"...").
const RoleEnum = "admin,manager,seller,checker,finance,driver"
