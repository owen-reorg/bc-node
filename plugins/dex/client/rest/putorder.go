package rest

import (
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/cosmos/cosmos-sdk/client/context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"

	"github.com/BiJie/BinanceChain/common/utils"
	"github.com/BiJie/BinanceChain/plugins/api/helpers"
	"github.com/BiJie/BinanceChain/plugins/dex/order"
	"github.com/BiJie/BinanceChain/plugins/dex/store"
	"github.com/BiJie/BinanceChain/wire"
)

// PutOrderReqHandler creates an http request handler to create a new order transaction and return its binary tx
func PutOrderReqHandler(cdc *wire.Codec, ctx context.CoreContext, accStoreName string) http.HandlerFunc {
	type formParams struct {
		address string
		pair    string
		side    string
		price   string
		qty     string
		tif     string
	}
	type response struct {
		OK       bool   `json:"ok"`
		OrderID  string `json:"order_id"`
		HexBytes string `json:"tx_to_sign"`
		Sequence int64  `json:"sequence"`
	}
	validateFormParams := func(params formParams) bool {
		// TODO: there might be a better way to do this
		if strings.TrimSpace(params.address) == "" {
			return false
		}
		if strings.TrimSpace(params.pair) == "" {
			return false
		}
		if strings.TrimSpace(params.side) == "" {
			return false
		}
		if strings.TrimSpace(params.price) == "" {
			return false
		}
		if strings.TrimSpace(params.qty) == "" {
			return false
		}
		return true
	}
	throw := func(w http.ResponseWriter, status int, err error) {
		w.WriteHeader(status)
		w.Write([]byte(err.Error()))
		return
	}
	accDecoder := authcmd.GetAccountDecoder(cdc)
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: collect chainID too
		ctx.ChainID = "bnbchain-1000"

		// parse application/x-www-form-urlencoded or multipart/form-data form params
		params := formParams{
			address: r.FormValue("address"),
			pair:    r.FormValue("pair"),
			side:    r.FormValue("side"),
			price:   r.FormValue("price"),
			qty:     r.FormValue("qty"),
			tif:     r.FormValue("tif"),
		}

		if !validateFormParams(params) {
			throw(w, http.StatusExpectationFailed, errors.New("validation failed"))
			return
		}

		// query account by address
		addr, err := sdk.AccAddressFromBech32(params.address)
		if err != nil {
			throw(w, http.StatusNotFound, err)
			return
		}
		accbz, err := ctx.QueryStore(auth.AddressStoreKey(addr), accStoreName)
		if err != nil {
			throw(w, http.StatusInternalServerError, err)
			return
		}
		// the query will return empty if there is no data for this account
		if len(accbz) == 0 {
			throw(w, http.StatusNotFound, err)
			return
		}

		// decode the account
		account, err := accDecoder(accbz)
		if err != nil {
			throw(w, http.StatusInternalServerError, err)
			return
		}

		err = store.ValidatePairSymbol(params.pair)
		if err != nil {
			throw(w, http.StatusNotFound, err)
			return
		}
		pair := strings.ToUpper(params.pair)

		price, err := utils.ParsePrice(params.price)
		if err != nil {
			throw(w, http.StatusInternalServerError, err)
			return
		}

		qty, err := utils.ParsePrice(params.qty)
		if err != nil {
			throw(w, http.StatusExpectationFailed, err)
			return
		}

		tif, err := order.TifStringToTifCode(params.tif)
		if err != nil {
			// default to GTC
			tif = -1
		}

		side, err := order.SideStringToSideCode(params.side)
		if err != nil {
			throw(w, http.StatusExpectationFailed, err)
			return
		}

		seq := account.GetSequence()
		id := order.GenerateOrderID(seq, addr)
		msg := order.NewNewOrderMsg(addr, id, side, pair, price, qty)

		if tif > -1 {
			msg.TimeInForce = tif
		}
		msgs := []sdk.Msg{msg}

		// build the tx
		txBytes, err := helpers.BuildUnsignedTx(ctx, account, msgs, cdc)
		if err != nil {
			throw(w, http.StatusInternalServerError, err)
			return
		}

		resp := response{
			OK:       true,
			OrderID:  msg.Id,
			Sequence: seq,
			HexBytes: hex.EncodeToString(*txBytes),
		}

		output, err := cdc.MarshalJSON(resp)
		if err != nil {
			throw(w, http.StatusInternalServerError, err)
			return
		}

		w.Write(output)
	}
}
