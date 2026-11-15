package lunixi

import (
	"context"
	"net/url"
	"regexp"
)

const trAddressPath = "/api/v1/reference/tr-address"

var (
	provinceCodePattern = regexp.MustCompile(`^(0[1-9]|[1-7][0-9]|8[01])$`)
	districtCodePattern = regexp.MustCompile(`^[0-9]{1,12}$`)
)

// ReferenceClient reads the Lunixi address dictionary for Türkiye (province,
// district, neighborhood). The codes fill an ADDRESS input field, for example
// the debtor address of a promissory-note payment: { provinceCode,
// districtCode, neighborhoodCode, line }. Data is { items: [{ code, name }] };
// an empty list means the data for that level is not published yet.
type ReferenceClient struct{ client *Client }

// ListProvinces lists provinces with their code (01–81).
func (r *ReferenceClient) ListProvinces(ctx context.Context) (*Response, error) {
	return r.client.Do(ctx, Request{Method: "GET", Path: trAddressPath + "/provinces"})
}

// ListDistricts lists the districts of a province (provinceCode 01–81).
func (r *ReferenceClient) ListDistricts(ctx context.Context, provinceCode string) (*Response, error) {
	if !provinceCodePattern.MatchString(provinceCode) {
		return nil, invalid("provinceCode must be a two-digit province code from 01 to 81")
	}
	return r.client.Do(ctx, Request{Method: "GET", Path: trAddressPath + "/districts", Query: url.Values{"provinceCode": {provinceCode}}})
}

// ListNeighborhoods lists the neighborhoods of a district (districtCode from
// ListDistricts).
func (r *ReferenceClient) ListNeighborhoods(ctx context.Context, districtCode string) (*Response, error) {
	if !districtCodePattern.MatchString(districtCode) {
		return nil, invalid("districtCode must be a district code (digits) from ListDistricts")
	}
	return r.client.Do(ctx, Request{Method: "GET", Path: trAddressPath + "/neighborhoods", Query: url.Values{"districtCode": {districtCode}}})
}
