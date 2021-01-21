package keeper

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/oraichain/orai/x/provider/types"
	"github.com/oraichain/orai/x/wasm"
	"github.com/tendermint/tendermint/libs/log"
)

// always clone keeper to make it immutable
type (
	Keeper struct {
		cdc        codec.Marshaler
		storeKey   sdk.StoreKey
		wasmKeeper wasm.Keeper
	}
)

func NewKeeper(cdc codec.Marshaler, storeKey sdk.StoreKey, wasmKeeper wasm.Keeper) *Keeper {
	return &Keeper{
		cdc:        cdc,
		storeKey:   storeKey,
		wasmKeeper: wasmKeeper,
	}
}

func (k *Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", fmt.Sprintf("x/%s", types.ModuleName))
}

// IsNamePresent checks if the name is present in the store or not
func (k *Keeper) IsNamePresent(ctx sdk.Context, name string) bool {
	store := ctx.KVStore(k.storeKey)
	return store.Has([]byte(name))
}

// GetMinimumFees collects minimum fees needed of an oracle script
func (k *Keeper) GetMinimumFees(ctx sdk.Context, dNames, tcNames []string, valNum int) (sdk.Coins, error) {
	var totalFees sdk.Coins
	// we have different test cases, so we need to loop through them
	for i := 0; i < len(tcNames); i++ {
		// loop to run the test case
		// collect all the test cases object to store in the ai request
		testCase, err := k.GetTestCase(ctx, tcNames[i])
		if err != nil {
			return nil, sdkerrors.Wrap(types.ErrTestCaseNotFound, fmt.Sprintf("failed to get test case: %s", err.Error()))
		}
		// Aggregate the required fees for an AI request
		totalFees = totalFees.Add(testCase.GetFees()...)
	}

	for j := 0; j < len(dNames); j++ {
		fmt.Println("data source: ", dNames[j])
		// collect all the data source objects to store in the ai request
		aiDataSource, err := k.GetAIDataSource(ctx, dNames[j])
		if err != nil {
			return nil, sdkerrors.Wrap(types.ErrDataSourceNotFound, fmt.Sprintf("failed to get data source: %s", err.Error()))
		}
		// Aggregate the required fees for an AI request
		totalFees = totalFees.Add(aiDataSource.GetFees()...)
	}
	// TODO:
	//rewardRatio := sdk.NewDecWithPrec(int64(k.GetParam(ctx, types.KeyOracleScriptRewardPercentage)), 2)
	rewardRatio := sdk.NewDecWithPrec(int64(60), 2)

	// check division by zero or negative figure
	if rewardRatio.IsZero() || rewardRatio.IsNegative() {
		rewardRatio = sdk.NewDecWithPrec(int64(60), 2)
	}
	//valFees = 2/5 total dsource and test case fees (70% total in 100% of total fees split into 20% and 50% respectively)
	valFees, _ := sdk.NewDecCoinsFromCoins(totalFees...).MulDec(sdk.NewDecWithPrec(int64(40), 2)).TruncateDecimal()
	//50% + 20% = 70% * validatorCount fees (since k validators will execute, the fees need to be propotional to the number of vals)
	bothFees := sdk.NewDecCoinsFromCoins(totalFees.Add(valFees...)...)
	finalFees, _ := bothFees.MulDec(sdk.NewDec(int64(valNum))).TruncateDecimal()
	minimumFees, _ := sdk.NewDecCoinsFromCoins(finalFees...).QuoDec(rewardRatio).TruncateDecimal()
	fmt.Println("minimum fees: ", minimumFees)
	return minimumFees, nil
}

// ############################################# Data source

// GetAllAIDataSourceNames get an iterator of all key-value pairs in the store
func (k *Keeper) GetAllAIDataSourceNames(ctx sdk.Context) sdk.Iterator {
	store := ctx.KVStore(k.storeKey)
	return sdk.KVStorePrefixIterator(store, []byte(types.DataSourceKeyPrefix))
}

// getPaginatedAIDataSourceNames get an iterator of paginated key-value pairs in the store
func (k *Keeper) getPaginatedAIDataSourceNames(ctx sdk.Context, page, limit uint) sdk.Iterator {
	store := ctx.KVStore(k.storeKey)
	return sdk.KVStorePrefixIteratorPaginated(store, []byte(types.DataSourceKeyPrefix), page, limit)
}

// GetAIDataSource returns the data source object given the name of the data source
func (k *Keeper) GetAIDataSource(ctx sdk.Context, name string) (*types.AIDataSource, error) {
	store := ctx.KVStore(k.storeKey)
	aiDataSource := &types.AIDataSource{}
	err := k.cdc.UnmarshalBinaryLengthPrefixed(store.Get(types.DataSourceStoreKey(name)), aiDataSource)
	return aiDataSource, err
}

// DefaultAIDataSource creates an empty Data Source struct
func (k Keeper) DefaultAIDataSource() types.AIDataSource {
	return types.AIDataSource{}
}

// GetAIDataSources returns list of data sources
func (k *Keeper) GetAIDataSources(ctx sdk.Context, page, limit uint) ([]types.AIDataSource, error) {
	var dSources []types.AIDataSource

	iterator := k.getPaginatedAIDataSourceNames(ctx, page, limit)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var dSource types.AIDataSource
		err := k.cdc.UnmarshalBinaryLengthPrefixed(iterator.Value(), &dSource)
		if err != nil {
			return []types.AIDataSource{}, err
		}
		dSources = append(dSources, dSource)
	}
	return dSources, nil
}

// SetAIDataSource allows users to set a data source into the store
func (k Keeper) SetAIDataSource(ctx sdk.Context, name string, aiDataSource *types.AIDataSource) {
	store := ctx.KVStore(k.storeKey)

	bz := k.cdc.MustMarshalBinaryLengthPrefixed(aiDataSource)
	store.Set(types.DataSourceStoreKey(name), bz)
}

// EditAIDataSource allows users to edit a data source in the store, just change address
func (k Keeper) EditAIDataSource(ctx sdk.Context, oldName, newName string, aiDataSource *types.AIDataSource) {
	key := types.DataSourceStoreKey(oldName)
	// if the user does not want to reuse the old name
	if oldName != newName {
		store := ctx.KVStore(k.storeKey)
		byteKey := []byte(key)
		store.Delete(byteKey)
	}
	k.SetAIDataSource(ctx, newName, aiDataSource)
}

// ###################################################### oracle script

// GetOracleScript returns the oScript object given the name of the oScript
func (k *Keeper) GetOracleScript(ctx sdk.Context, name string) (*types.OracleScript, error) {
	store := ctx.KVStore(k.storeKey)
	oScript := &types.OracleScript{}
	err := k.cdc.UnmarshalBinaryLengthPrefixed(store.Get(types.OracleScriptStoreKey(name)), oScript)
	return oScript, err
}

// SetOracleScript allows users to set a oScript into the store
func (k Keeper) SetOracleScript(ctx sdk.Context, name string, oScript *types.OracleScript) {
	store := ctx.KVStore(k.storeKey)
	bz := k.cdc.MustMarshalBinaryLengthPrefixed(oScript)
	store.Set(types.OracleScriptStoreKey(name), bz)
}

// GetOracleScripts returns list of oracle scripts
func (k *Keeper) GetOracleScripts(ctx sdk.Context, page, limit uint) ([]types.OracleScript, error) {
	var oScripts []types.OracleScript

	iterator := k.GetPaginatedOracleScriptNames(ctx, page, limit)
	//iterator := k.GetAllOracleScriptNames(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var oScript types.OracleScript
		err := k.cdc.UnmarshalBinaryLengthPrefixed(iterator.Value(), &oScript)
		if err != nil {
			return []types.OracleScript{}, err
		}
		oScripts = append(oScripts, oScript)
	}
	return oScripts, nil
}

// GetAllOracleScriptNames get an iterator of all key-value pairs in the store
func (k *Keeper) GetAllOracleScriptNames(ctx sdk.Context) sdk.Iterator {
	store := ctx.KVStore(k.storeKey)
	return sdk.KVStorePrefixIterator(store, []byte(types.OScriptKeyPrefix))
}

// GetPaginatedOracleScriptNames get an iterator of paginated key-value pairs in the store
func (k *Keeper) GetPaginatedOracleScriptNames(ctx sdk.Context, page, limit uint) sdk.Iterator {
	store := ctx.KVStore(k.storeKey)
	return sdk.KVStorePrefixIteratorPaginated(store, []byte(types.OScriptKeyPrefix), page, limit)
}

// EditOracleScript allows users to edit a oScript in the store
func (k Keeper) EditOracleScript(ctx sdk.Context, oldName, newName string, oScript *types.OracleScript) {

	key := types.OracleScriptStoreKey(oldName)
	// if the user does not want to reuse the old name
	if oldName != newName {
		store := ctx.KVStore(k.storeKey)
		byteKey := []byte(key)
		store.Delete(byteKey)

	}
	k.SetOracleScript(ctx, newName, oScript)
}

// GetDNamesTcNames - an utility function for retriving data source and test case names from the oracle script
func (k *Keeper) GetDNamesTcNames(ctx sdk.Context, oScript string) ([]string, []string, error) {
	// get data source and test case names from the oracle script
	oracleScript, err := k.GetOracleScript(ctx, oScript)
	if err != nil {
		return nil, nil, err
	}
	aiDataSources := oracleScript.GetDSources()
	testCases := oracleScript.GetTCases()
	return aiDataSources, testCases, nil
}

// GetKeyOracleScriptRewardPercentage returns the oracle script reward percentage from the provider module
func (k *Keeper) GetKeyOracleScriptRewardPercentage(ctx sdk.Context) int64 {
	// TODO
	//percentage := k.GetParam(ctx, types.KeyOracleScriptRewardPercentage)
	return int64(60)
}

// #################################################### Test case

// GetAllTestCaseNames get an iterator of all key-value pairs in the store
func (k *Keeper) GetAllTestCaseNames(ctx sdk.Context) sdk.Iterator {
	store := ctx.KVStore(k.storeKey)
	return sdk.KVStorePrefixIterator(store, []byte(types.TestCaseKeyPrefix))
}

// GetPaginatedTestCaseNames get an iterator of paginated key-value pairs in the store
func (k *Keeper) GetPaginatedTestCaseNames(ctx sdk.Context, page, limit uint) sdk.Iterator {
	store := ctx.KVStore(k.storeKey)
	return sdk.KVStorePrefixIteratorPaginated(store, []byte(types.TestCaseKeyPrefix), page, limit)
}

// GetTestCase returns the the AI test case of a given request
func (k *Keeper) GetTestCase(ctx sdk.Context, name string) (*types.TestCase, error) {
	store := ctx.KVStore(k.storeKey)
	testCase := &types.TestCase{}
	err := k.cdc.UnmarshalBinaryLengthPrefixed(store.Get(types.TestCaseStoreKey(name)), testCase)
	return testCase, err
}

// GetTestCases returns list of test cases
func (k *Keeper) GetTestCases(ctx sdk.Context, page, limit uint) ([]types.TestCase, error) {
	var tCases []types.TestCase

	iterator := k.GetPaginatedTestCaseNames(ctx, page, limit)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var tCase types.TestCase
		err := k.cdc.UnmarshalBinaryLengthPrefixed(iterator.Value(), &tCase)
		if err != nil {
			return []types.TestCase{}, err
		}
		tCases = append(tCases, tCase)
	}
	return tCases, nil
}

// SetTestCase allows users to set a test case into the store
func (k Keeper) SetTestCase(ctx sdk.Context, name string, testCase *types.TestCase) {
	store := ctx.KVStore(k.storeKey)

	bz := k.cdc.MustMarshalBinaryLengthPrefixed(testCase)
	store.Set(types.TestCaseStoreKey(name), bz)
}

// DefaultTestCase creates an empty Test Case struct
func (k Keeper) DefaultTestCase() types.TestCase {
	return types.TestCase{}
}

// EditTestCase allows users to edit a test case in the store
func (k Keeper) EditTestCase(ctx sdk.Context, oldName, newName string, testCase *types.TestCase) {
	key := types.TestCaseStoreKey(oldName)
	// if the user does not want to reuse the old name
	if oldName != newName {
		store := ctx.KVStore(k.storeKey)
		byteKey := []byte(key)
		store.Delete(byteKey)
	}
	k.SetTestCase(ctx, newName, testCase)
}
