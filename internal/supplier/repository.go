package supplier

import "context"

// Repository est défini par le domaine ; supplierstore l'implémente avec pgx/sqlc.
type Repository interface {
	Supplier(context.Context, int64) (Supplier, error)
	ListSuppliers(context.Context) ([]Supplier, error)
	Contract(context.Context, int64) (Contract, error)
	ListContracts(context.Context, int64) ([]Contract, error)
	ListOfferContracts(context.Context, int64) ([]Contract, error)
	// ListAllContracts : tous les contrats, par fournisseur puis par id.
	ListAllContracts(context.Context) ([]Contract, error)
	WithinTx(context.Context, func(TxRepository) error) error
}

type TxRepository interface {
	Supplier(context.Context, int64) (Supplier, error)
	SupplierForUpdate(context.Context, int64) (Supplier, error)
	ContractForUpdate(context.Context, int64) (Contract, error)
	InsertSupplier(context.Context, Supplier) (Supplier, error)
	ReplaceSupplier(context.Context, Supplier) (Supplier, error)
	InsertContract(context.Context, Contract) (Contract, error)
	ReplaceContract(context.Context, Contract) (Contract, error)
	// CheckOffers verrouille toutes les pièces par id puis les offres par id sous FOR SHARE.
	// Il vérifie existence, archives et appartenance au fournisseur dans la transaction courante.
	CheckOffers(context.Context, int64, []int64) error
}
