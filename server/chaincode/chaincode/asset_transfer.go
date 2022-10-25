/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

const assetCollection = "assetCollection"
const transferAgreementObjectType = "transferAgreement"

// SmartContract of this fabric sample
type SmartContract struct {
	contractapi.Contract
}

// Asset describes main asset details that are visible to all organizations
type Asset struct {
	Type    string `json:"assetType"` //Type is used to distinguish the various types of objects in state database
	ID      string `json:"licenseID"`
	Details string `json:"licenseDetails"`
	Creator string `json:"creatorID"`
	Owner   string `json:"ownerID"`
}

// AssetPrivateDetails describes details that are private to owners
type AssetPrivateDetails struct {
	ID             string `json:"assetID"`
	AppraisedValue int    `json:"appraisedValue"`
}

// TransferAgreement describes the buyer agreement returned by ReadTransferAgreement
type TransferAgreement struct {
	ID      string `json:"assetID"`
	BuyerID string `json:"buyerID"`
}

// CreateAsset creates a new asset by placing the main asset details in the assetCollection
// that can be read by both organizations. The appraisal value is stored in the owners org specific collection.
func (s *SmartContract) CreateAsset(ctx contractapi.TransactionContextInterface, assetID, licenseDetails string) error {

	exists, err := s.AssetExists(ctx, assetID)
	if err != nil {
		return fmt.Errorf("failed to get asset: %v", err)
	}
	if exists {
		return fmt.Errorf("asset already exists: %s", assetID)
	}

	// Get ID of submitting client identity
	clientID, err := submittingClientIdentity(ctx)
	if err != nil {
		return err
	}

	// Verify that the client is submitting request to peer in their organization
	// This is to ensure that a client from another org doesn't attempt to read or
	// write private data from this peer.
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("CreateAsset cannot be performed: Error %v", err)
	}

	asset := &Asset{
		Type:    "asset",
		ID:      assetID,
		Details: licenseDetails,
		Creator: clientID,
		Owner:   clientID,
	}
	assetBytes, err := json.Marshal(asset)
	if err != nil {
		return err
	}

	if err != nil {
		return fmt.Errorf("failed to marshal asset into JSON: %v", err)
	}

	err = ctx.GetStub().PutState(assetID, assetBytes)
	if err != nil {
		return err
	}

	// Save asset to private data collection
	// Typical logger, logs to stdout/file in the fabric managed docker container, running this chaincode
	// Look for container name like dev-peer0.org1.example.com-{chaincodename_version}-xyz
	// log.Printf("CreateAsset Put: collection %v, ID %v, owner %v", assetCollection, assetInput.ID, clientID)

	// err = ctx.GetStub().PutPrivateData(assetCollection, assetInput.ID, assetJSONasBytes)
	// if err != nil {
	// 	return fmt.Errorf("failed to put asset into private data collecton: %v", err)
	// }

	// // Save asset details to collection visible to owning organization
	// assetPrivateDetails := AssetPrivateDetails{
	// 	ID:             assetInput.ID,
	// 	AppraisedValue: assetInput.AppraisedValue,
	// }

	// assetPrivateDetailsAsBytes, err := json.Marshal(assetPrivateDetails) // marshal asset details to JSON
	// if err != nil {
	// 	return fmt.Errorf("failed to marshal into JSON: %v", err)
	// }

	// // Get collection name for this organization.
	// orgCollection, err := getCollectionName(ctx)
	// if err != nil {
	// 	return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	// }

	// // Put asset appraised value into owners org specific private data collection
	// log.Printf("Put: collection %v, ID %v", orgCollection, assetInput.ID)
	// err = ctx.GetStub().PutPrivateData(orgCollection, assetInput.ID, assetPrivateDetailsAsBytes)
	// if err != nil {
	// 	return fmt.Errorf("failed to put asset private details: %v", err)
	// }
	return nil
}

// AgreeToTransfer is used by the potential buyer of the asset to agree to the
// asset value. The agreed to appraisal value is stored in the buying orgs
// org specifc collection, while the the buyer client ID is stored in the asset collection
// using a composite key
func (s *SmartContract) AgreeToTransfer(ctx contractapi.TransactionContextInterface) error {

	// Get ID of submitting client identity
	clientID, err := submittingClientIdentity(ctx)
	if err != nil {
		return err
	}

	// Value is private, therefore it gets passed in transient field
	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("error getting transient: %v", err)
	}

	// Persist the JSON bytes as-is so that there is no risk of nondeterministic marshaling.
	valueJSONasBytes, ok := transientMap["asset_value"]
	if !ok {
		return fmt.Errorf("asset_value key not found in the transient map")
	}

	// Unmarshal the tranisent map to get the asset ID.
	var valueJSON AssetPrivateDetails
	err = json.Unmarshal(valueJSONasBytes, &valueJSON)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	// Do some error checking since we get the chance
	if len(valueJSON.ID) == 0 {
		return fmt.Errorf("assetID field must be a non-empty string")
	}
	if valueJSON.AppraisedValue <= 0 {
		return fmt.Errorf("appraisedValue field must be a positive integer")
	}

	// Read asset from ledger
	asset, err := s.ReadAsset(ctx, valueJSON.ID)
	if err != nil {
		return fmt.Errorf("error reading asset: %v", err)
	}
	if asset == nil {
		return fmt.Errorf("%v does not exist", valueJSON.ID)
	}
	// Verify that the client is submitting request to peer in their organization
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("AgreeToTransfer cannot be performed: Error %v", err)
	}

	// Get collection name for this organization. Needs to be read by a member of the organization.
	orgCollection, err := getCollectionName(ctx)
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}

	log.Printf("AgreeToTransfer Put: collection %v, ID %v", orgCollection, valueJSON.ID)

	// Put agreed value in the org specifc private data collection
	err = ctx.GetStub().PutPrivateData(orgCollection, valueJSON.ID, valueJSONasBytes)
	if err != nil {
		return fmt.Errorf("failed to put asset bid: %v", err)
	}

	// If the one calling this transaction is not the asset owner,
	// Create agreeement that indicates which identity has agreed to purchase
	// In a more realistic transfer scenario, a transfer agreement would be secured to ensure that it cannot
	// be overwritten by another channel member
	if clientID != asset.Owner {
		transferAgreeKey, err := ctx.GetStub().CreateCompositeKey(transferAgreementObjectType, []string{valueJSON.ID})
		if err != nil {
			return fmt.Errorf("failed to create composite key: %v", err)
		}

		log.Printf("AgreeToTransfer Put: collection %v, ID %v, Key %v", assetCollection, valueJSON.ID, transferAgreeKey)
		err = ctx.GetStub().PutPrivateData(assetCollection, transferAgreeKey, []byte(clientID))
		if err != nil {
			return fmt.Errorf("failed to put asset bid: %v", err)
		}
	}

	return nil
}

// TransferAsset transfers the asset to the new owner by setting a new owner ID
func (s *SmartContract) TransferAsset(ctx contractapi.TransactionContextInterface, assetID, buyerMSP string) error {

	// transientMap, err := ctx.GetStub().GetTransient()
	// if err != nil {
	// 	return fmt.Errorf("error getting transient %v", err)
	// }

	// // Asset properties are private, therefore they get passed in transient field
	// transientTransferJSON, ok := transientMap["asset_owner"]
	// if !ok {
	// 	return fmt.Errorf("asset owner not found in the transient map")
	// }

	// type assetTransferTransientInput struct {
	// 	ID       string `json:"assetID"`
	// 	BuyerMSP string `json:"buyerMSP"`
	// }

	// var assetTransferInput assetTransferTransientInput
	// err = json.Unmarshal(transientTransferJSON, &assetTransferInput)
	// if err != nil {
	// 	return fmt.Errorf("failed to unmarshal JSON: %v", err)
	// }

	if len(assetID) == 0 {
		return fmt.Errorf("assetID field must be a non-empty string")
	}
	if len(buyerMSP) == 0 {
		return fmt.Errorf("buyerMSP field must be a non-empty string")
	}
	log.Printf("TransferAsset: verify asset exists ID %v", assetID)
	// Read asset from the private data collection
	asset, err := s.ReadAsset(ctx, assetID)
	if err != nil {
		return fmt.Errorf("error reading asset: %v", err)
	}
	if asset == nil {
		return fmt.Errorf("%v does not exist", assetID)
	}
	// Verify that the client is submitting request to peer in their organization
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("TransferAsset cannot be performed: Error %v", err)
	}

	// Verify transfer details and transfer owner
	err = s.verifyAgreement(ctx, assetID, asset.Owner, buyerMSP)
	if err != nil {
		return fmt.Errorf("failed transfer verification: %v", err)
	}

	transferAgreement, err := s.ReadTransferAgreement(ctx, assetID)
	if err != nil {
		return fmt.Errorf("failed ReadTransferAgreement to find buyerID: %v", err)
	}
	if transferAgreement.BuyerID == "" {
		return fmt.Errorf("BuyerID not found in TransferAgreement for %v", assetID)
	}

	// Transfer asset in ledger to new owner
	asset.Owner = transferAgreement.BuyerID

	assetBytes, err := json.Marshal(asset)
	if err != nil {
		return err
	}

	if err != nil {
		return fmt.Errorf("failed to marshal asset into JSON: %v", err)
	}

	err = ctx.GetStub().PutState(assetID, assetBytes)
	if err != nil {
		return err
	}

	// Get collection name for this organization
	ownersCollection, err := getCollectionName(ctx)
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}

	// Delete the asset appraised value from this organization's private data collection
	err = ctx.GetStub().DelPrivateData(ownersCollection, assetID)
	if err != nil {
		return err
	}

	// Delete the transfer agreement from the asset collection
	transferAgreeKey, err := ctx.GetStub().CreateCompositeKey(transferAgreementObjectType, []string{assetID})
	if err != nil {
		return fmt.Errorf("failed to create composite key: %v", err)
	}

	err = ctx.GetStub().DelPrivateData(assetCollection, transferAgreeKey)
	if err != nil {
		return err
	}

	return nil

}

// verifyAgreement is an internal helper function used by TransferAsset to verify
// that the transfer is being initiated by the owner and that the buyer has agreed
// to the same appraisal value as the owner
func (s *SmartContract) verifyAgreement(ctx contractapi.TransactionContextInterface, assetID string, ownerID string, buyerMSP string) error {

	// Check 1: verify that the transfer is being initiatied by the owner

	// Get ID of submitting client identity
	clientID, err := submittingClientIdentity(ctx)
	if err != nil {
		return err
	}

	if clientID != ownerID {
		return fmt.Errorf("error: submitting client identity does not own asset")
	}

	// Check 2: verify that the buyer has agreed to the appraised value

	// Get collection names
	collectionOwner, err := getCollectionName(ctx) // get owner collection from caller identity
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}

	collectionBuyer := buyerMSP + "PrivateCollection" // get buyers collection

	// Get hash of owners agreed to value
	ownerAppraisedValueHash, err := ctx.GetStub().GetPrivateDataHash(collectionOwner, assetID)
	if err != nil {
		return fmt.Errorf("failed to get hash of appraised value from owners collection %v: %v", collectionOwner, err)
	}
	if ownerAppraisedValueHash == nil {
		return fmt.Errorf("hash of appraised value for %v does not exist in collection %v", assetID, collectionOwner)
	}

	// Get hash of buyers agreed to value
	buyerAppraisedValueHash, err := ctx.GetStub().GetPrivateDataHash(collectionBuyer, assetID)
	if err != nil {
		return fmt.Errorf("failed to get hash of appraised value from buyer collection %v: %v", collectionBuyer, err)
	}
	if buyerAppraisedValueHash == nil {
		return fmt.Errorf("hash of appraised value for %v does not exist in collection %v. AgreeToTransfer must be called by the buyer first", assetID, collectionBuyer)
	}

	// Verify that the two hashes match
	if !bytes.Equal(ownerAppraisedValueHash, buyerAppraisedValueHash) {
		return fmt.Errorf("hash for appraised value for owner %x does not value for seller %x", ownerAppraisedValueHash, buyerAppraisedValueHash)
	}

	return nil
}

// DeleteAsset can be used by the owner of the asset to delete the asset
func (s *SmartContract) DeleteAsset(ctx contractapi.TransactionContextInterface) error {

	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("Error getting transient: %v", err)
	}

	// Asset properties are private, therefore they get passed in transient field
	transientDeleteJSON, ok := transientMap["asset_delete"]
	if !ok {
		return fmt.Errorf("asset to delete not found in the transient map")
	}

	type assetDelete struct {
		ID string `json:"assetID"`
	}

	var assetDeleteInput assetDelete
	err = json.Unmarshal(transientDeleteJSON, &assetDeleteInput)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	if len(assetDeleteInput.ID) == 0 {
		return fmt.Errorf("assetID field must be a non-empty string")
	}

	// Verify that the client is submitting request to peer in their organization
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("DeleteAsset cannot be performed: Error %v", err)
	}

	log.Printf("Deleting Asset: %v", assetDeleteInput.ID)
	valAsbytes, err := ctx.GetStub().GetPrivateData(assetCollection, assetDeleteInput.ID) //get the asset from chaincode state
	if err != nil {
		return fmt.Errorf("failed to read asset: %v", err)
	}
	if valAsbytes == nil {
		return fmt.Errorf("asset not found: %v", assetDeleteInput.ID)
	}

	ownerCollection, err := getCollectionName(ctx) // Get owners collection
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}

	//check the asset is in the caller org's private collection
	valAsbytes, err = ctx.GetStub().GetPrivateData(ownerCollection, assetDeleteInput.ID)
	if err != nil {
		return fmt.Errorf("failed to read asset from owner's Collection: %v", err)
	}
	if valAsbytes == nil {
		return fmt.Errorf("asset not found in owner's private Collection %v: %v", ownerCollection, assetDeleteInput.ID)
	}

	// delete the asset from state
	err = ctx.GetStub().DelPrivateData(assetCollection, assetDeleteInput.ID)
	if err != nil {
		return fmt.Errorf("failed to delete state: %v", err)
	}

	// Finally, delete private details of asset
	err = ctx.GetStub().DelPrivateData(ownerCollection, assetDeleteInput.ID)
	if err != nil {
		return err
	}

	return nil

}

// DeleteTranferAgreement can be used by the buyer to withdraw a proposal from
// the asset collection and from his own collection.
func (s *SmartContract) DeleteTranferAgreement(ctx contractapi.TransactionContextInterface) error {

	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("error getting transient: %v", err)
	}

	// Asset properties are private, therefore they get passed in transient field
	transientDeleteJSON, ok := transientMap["agreement_delete"]
	if !ok {
		return fmt.Errorf("asset to delete not found in the transient map")
	}

	type assetDelete struct {
		ID string `json:"assetID"`
	}

	var assetDeleteInput assetDelete
	err = json.Unmarshal(transientDeleteJSON, &assetDeleteInput)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	if len(assetDeleteInput.ID) == 0 {
		return fmt.Errorf("transient input ID field must be a non-empty string")
	}

	// Verify that the client is submitting request to peer in their organization
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("DeleteTranferAgreement cannot be performed: Error %v", err)
	}
	// Delete private details of agreement
	orgCollection, err := getCollectionName(ctx) // Get proposers collection.
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}
	tranferAgreeKey, err := ctx.GetStub().CreateCompositeKey(transferAgreementObjectType, []string{assetDeleteInput.
		ID}) // Create composite key
	if err != nil {
		return fmt.Errorf("failed to create composite key: %v", err)
	}

	valAsbytes, err := ctx.GetStub().GetPrivateData(assetCollection, tranferAgreeKey) //get the transfer_agreement
	if err != nil {
		return fmt.Errorf("failed to read transfer_agreement: %v", err)
	}
	if valAsbytes == nil {
		return fmt.Errorf("asset's transfer_agreement does not exist: %v", assetDeleteInput.ID)
	}

	log.Printf("Deleting TranferAgreement: %v", assetDeleteInput.ID)
	err = ctx.GetStub().DelPrivateData(orgCollection, assetDeleteInput.ID) // Delete the asset
	if err != nil {
		return err
	}

	// Delete transfer agreement record
	err = ctx.GetStub().DelPrivateData(assetCollection, tranferAgreeKey) // remove agreement from state
	if err != nil {
		return err
	}

	return nil

}

// getCollectionName is an internal helper function to get collection of submitting client identity.
func getCollectionName(ctx contractapi.TransactionContextInterface) (string, error) {

	// Get the MSP ID of submitting client identity
	clientMSPID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", fmt.Errorf("failed to get verified MSPID: %v", err)
	}

	// Create the collection name
	orgCollection := clientMSPID + "PrivateCollection"

	return orgCollection, nil
}

// verifyClientOrgMatchesPeerOrg is an internal function used verify client org id and matches peer org id.
func verifyClientOrgMatchesPeerOrg(ctx contractapi.TransactionContextInterface) error {
	clientMSPID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("failed getting the client's MSPID: %v", err)
	}
	peerMSPID, err := shim.GetMSPID()
	if err != nil {
		return fmt.Errorf("failed getting the peer's MSPID: %v", err)
	}

	if clientMSPID != peerMSPID {
		return fmt.Errorf("client from org %v is not authorized to read or write private data from an org %v peer", clientMSPID, peerMSPID)
	}

	return nil
}

func submittingClientIdentity(ctx contractapi.TransactionContextInterface) (string, error) {
	b64ID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("Failed to read clientID: %v", err)
	}
	decodeID, err := base64.StdEncoding.DecodeString(b64ID)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode clientID: %v", err)
	}
	return string(decodeID), nil
}

// AssetExists returns true when asset with given ID exists in the ledger.
func (s *SmartContract) AssetExists(ctx contractapi.TransactionContextInterface, assetID string) (bool, error) {
	assetBytes, err := ctx.GetStub().GetState(assetID)
	if err != nil {
		return false, fmt.Errorf("failed to read asset %s from world state. %v", assetID, err)
	}

	return assetBytes != nil, nil
}
