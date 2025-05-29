package webhooks

import (
	"encoding/json"
	"fmt"
	"net/http"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	infrav1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

// Beskar7ConversionWebhook implements a conversion webhook for Beskar7Cluster and Beskar7Machine.
type Beskar7ConversionWebhook struct {
	scheme *runtime.Scheme
}

// NewBeskar7ConversionWebhook creates a new Beskar7ConversionWebhook.
func NewBeskar7ConversionWebhook(scheme *runtime.Scheme) *Beskar7ConversionWebhook {
	return &Beskar7ConversionWebhook{
		scheme: scheme,
	}
}

// ServeHTTP implements http.Handler.
func (webhook *Beskar7ConversionWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var review apiextensionsv1.ConversionReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		http.Error(w, fmt.Sprintf("could not decode body: %v", err), http.StatusBadRequest)
		return
	}

	response := webhook.handleConversion(review)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

func (webhook *Beskar7ConversionWebhook) handleConversion(review apiextensionsv1.ConversionReview) apiextensionsv1.ConversionReview {
	response := apiextensionsv1.ConversionReview{
		TypeMeta: review.TypeMeta,
		Response: &apiextensionsv1.ConversionResponse{
			UID:              review.Request.UID,
			ConvertedObjects: make([]runtime.RawExtension, len(review.Request.Objects)),
			Result: metav1.Status{
				Status: "Success",
			},
		},
	}

	for i, obj := range review.Request.Objects {
		converted, err := webhook.convertObject(obj.Raw, review.Request.DesiredAPIVersion)
		if err != nil {
			response.Response.Result = metav1.Status{
				Status:  "Failure",
				Message: fmt.Sprintf("failed to convert object %d: %v", i, err),
			}
			break
		}
		response.Response.ConvertedObjects[i] = runtime.RawExtension{
			Raw: converted,
		}
	}

	return response
}

func (webhook *Beskar7ConversionWebhook) convertObject(raw []byte, targetVersion string) ([]byte, error) {
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal object: %v", err)
	}

	switch obj.GetKind() {
	case "Beskar7Cluster":
		return webhook.convertBeskar7Cluster(obj, targetVersion)
	case "Beskar7Machine":
		return webhook.convertBeskar7Machine(obj, targetVersion)
	default:
		return nil, fmt.Errorf("unsupported kind: %s", obj.GetKind())
	}
}

func (webhook *Beskar7ConversionWebhook) convertBeskar7Cluster(obj *unstructured.Unstructured, targetVersion string) ([]byte, error) {
	if targetVersion != infrav1beta1.GroupVersion.String() {
		return nil, fmt.Errorf("unsupported target version: %s", targetVersion)
	}

	// Since we're only supporting v1beta1, we can just return the object as is
	return json.Marshal(obj.Object)
}

func (webhook *Beskar7ConversionWebhook) convertBeskar7Machine(obj *unstructured.Unstructured, targetVersion string) ([]byte, error) {
	if targetVersion != infrav1beta1.GroupVersion.String() {
		return nil, fmt.Errorf("unsupported target version: %s", targetVersion)
	}

	// Since we're only supporting v1beta1, we can just return the object as is
	return json.Marshal(obj.Object)
}
