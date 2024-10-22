package usEskimi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/prebid/openrtb/v20/openrtb2"
	"github.com/prebid/prebid-server/v2/adapters"
	"github.com/prebid/prebid-server/v2/config"
	"github.com/prebid/prebid-server/v2/errortypes"
	"github.com/prebid/prebid-server/v2/openrtb_ext"
)

const (
	meditaTypeParam = "{MediaType}"
)

type ImpExtBidder struct {
	Bidder struct {
		MediaType string `json:"mediaType"`
	}
}

type adapter struct {
	endpoint string
}

type reqBodyExt struct {
	UsEskimiBidderExt reqBodyExtBidder `json:"bidder"`
}

type reqBodyExtBidder struct {
	Type        string `json:"type"`
	PlacementID string `json:"placementId,omitempty"`
}

func Builder(bidderName openrtb_ext.BidderName, config config.Adapter, server config.Server) (adapters.Bidder, error) {
	bidder := &adapter{
		endpoint: config.Endpoint,
	}
	return bidder, nil
}

func (a *adapter) MakeRequests(request *openrtb2.BidRequest, reqInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	adapterRequests := make([]*adapters.RequestData, 0, len(request.Imp))

	reqCopy := *request
	for _, imp := range request.Imp {
		reqCopy.Imp = []openrtb2.Imp{imp}

		placementID, err := jsonparser.GetString(imp.Ext, "bidder", "placementId")
		if err != nil {
			return nil, []error{err}
		}

		extJson, err := json.Marshal(reqBodyExt{
			UsEskimiBidderExt: reqBodyExtBidder{
				PlacementID: placementID,
				Type:        "publisher",
			},
		})
		if err != nil {
			return nil, []error{err}
		}

		reqCopy.Imp[0].Ext = extJson

		adapterReq, err := a.makeRequest(&reqCopy, imp)
		if err != nil {
			return nil, []error{err}
		}

		if adapterReq != nil {
			adapterRequests = append(adapterRequests, adapterReq)
		}
	}
	return adapterRequests, nil
}

func (a *adapter) makeRequest(request *openrtb2.BidRequest, imp openrtb2.Imp) (*adapters.RequestData, error) {
	reqJSON, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Add("Content-Type", "application/json;charset=utf-8")
	headers.Add("Accept", "application/json")
	return &adapters.RequestData{
		Method:  "POST",
		Uri:     a.buildEndpointURL(imp),
		Body:    reqJSON,
		Headers: headers,
		ImpIDs:  openrtb_ext.GetImpIDs(request.Imp),
	}, err
}

func (a *adapter) MakeBids(request *openrtb2.BidRequest, requestData *adapters.RequestData, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if responseData.StatusCode != http.StatusOK {
		err := &errortypes.BadServerResponse{
			Message: fmt.Sprintf("Unexpected status code: %d. Run with request.debug = 1 for more info.", responseData.StatusCode),
		}
		return nil, []error{err}
	}

	var response openrtb2.BidResponse
	if err := json.Unmarshal(responseData.Body, &response); err != nil {
		return nil, []error{err}
	}

	bidResponse := adapters.NewBidderResponseWithBidsCapacity(len(request.Imp))
	bidResponse.Currency = response.Cur
	for _, seatBid := range response.SeatBid {
		for i := range seatBid.Bid {
			bidType, err := getMediaTypeForImp(seatBid.Bid[i].ImpID, request.Imp)
			if err != nil {
				return nil, []error{err}
			}

			b := &adapters.TypedBid{
				Bid:     &seatBid.Bid[i],
				BidType: bidType,
			}
			bidResponse.Bids = append(bidResponse.Bids, b)
		}
	}
	return bidResponse, nil
}

func getMediaTypeForImp(impID string, imps []openrtb2.Imp) (openrtb_ext.BidType, error) {
	for _, imp := range imps {
		if imp.ID == impID {
			if imp.Banner != nil {
				return openrtb_ext.BidTypeBanner, nil
			}
			if imp.Video != nil {
				return openrtb_ext.BidTypeVideo, nil
			}
			if imp.Native != nil {
				return openrtb_ext.BidTypeNative, nil
			}
		}
	}

	return "", &errortypes.BadInput{
		Message: fmt.Sprintf("Failed to find impression \"%s\"", impID),
	}
}

// func (a *adapter) buildEndpointURL(imp openrtb2.Imp) string {
// 	publisherEndpoint := ""
// 	var impBidder ImpExtBidder

// 	err := json.Unmarshal(imp.Ext, &impBidder)
// 	if err == nil && impBidder.Bidder.MediaType != "" {
// 		publisherEndpoint = strconv.Itoa(impBidder.Bidder.MediaType) + "/"
// 	}

// 	return strings.Replace(a.endpoint, meditaTypeParam, publisherEndpoint, -1)
// }

func (a *adapter) buildEndpointURL(imp openrtb2.Imp) string {
	// Initialize the publisherEndpoint variable
	publisherEndpoint := ""
	var impBidder ImpExtBidder

	// Unmarshal the extension field from the imp struct
	err := json.Unmarshal(imp.Ext, &impBidder)
	if err == nil && impBidder.Bidder.MediaType != "" {
		// Use MediaType directly as a string, no need to convert to int
		publisherEndpoint = impBidder.Bidder.MediaType
	}

	// Replace the placeholder in the endpoint URL with the actual publisherEndpoint
	return strings.Replace(a.endpoint, meditaTypeParam, publisherEndpoint, -1)
}
