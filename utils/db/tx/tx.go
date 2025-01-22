package tx

import (
	"context"
	"database/sql"
	"github.com/go-jet/jet/v2/qrm"
	"jummechu-api/main/common/db"
)

type TxExtension struct {
	Postgresql *db.Database
}

func (p TxExtension) GetTx(ctx context.Context) qrm.DB {
	tx := ctx.Value("tx")
	if tx != nil {
		result, ok := tx.(*sql.Tx)
		if !ok {
			return p.Postgresql.DbForJet
		}
		return result
	} else {
		return p.Postgresql.DbForJet
	}
}
