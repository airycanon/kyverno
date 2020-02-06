package engine

import (
	"encoding/json"
	"time"

	"github.com/nirmata/kyverno/pkg/engine/rbac"

	"github.com/golang/glog"

	"github.com/minio/minio/pkg/wildcard"
	kyverno "github.com/nirmata/kyverno/pkg/api/kyverno/v1"
	"github.com/nirmata/kyverno/pkg/engine/context"
	"github.com/nirmata/kyverno/pkg/engine/response"
	"github.com/nirmata/kyverno/pkg/engine/variables"
	"github.com/nirmata/kyverno/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

//EngineStats stores in the statistics for a single application of resource
type EngineStats struct {
	// average time required to process the policy rules on a resource
	ExecutionTime time.Duration
	// Count of rules that were applied successfully
	RulesAppliedCount int
}

//MatchesResourceDescription checks if the resource matches resource desription of the rule or not
func MatchesResourceDescription(resource unstructured.Unstructured, rule kyverno.Rule, admissionInfo kyverno.RequestInfo) bool {

	var condition = make(chan bool)
	defer close(condition)

	matches := rule.MatchResources.ResourceDescription

	go func() {
		hasSuceeded := rbac.MatchAdmissionInfo(rule, admissionInfo)
		if !hasSuceeded {
			glog.V(3).Infof("rule '%s' cannot be applied on %s/%s/%s, admission permission: %v",
				rule.Name, resource.GetKind(), resource.GetNamespace(), resource.GetName(), admissionInfo)
		}
		condition <- hasSuceeded
	}()

	go func() {
		condition <- findKind(matches.Kinds, resource.GetKind())
	}()

	name := resource.GetName()
	namespace := resource.GetNamespace()

	go func() {
		if matches.Name != "" {
			// Matches
			condition <- wildcard.Match(matches.Name, name)
		}
	}()

	// Matches
	// check if the resource namespace is defined in the list of namespace pattern
	go func() {
		condition <- !(len(matches.Namespaces) > 0 && !utils.ContainsNamepace(matches.Namespaces, namespace))
	}()

	// Matches
	go func() {
		condition <- func() bool {
			if matches.Selector != nil {
				selector, err := metav1.LabelSelectorAsSelector(matches.Selector)
				if err != nil {
					glog.Error(err)
					return false
				}
				if !selector.Matches(labels.Set(resource.GetLabels())) {
					return false
				}
			}
			return true
		}()
	}()

	//
	//
	//
	// Exclude Conditions
	//
	//
	//

	exclude := rule.ExcludeResources.ResourceDescription

	go func() {
		condition <- func() bool {
			if exclude.Name == "" {
				return true
			}
			if wildcard.Match(exclude.Name, resource.GetName()) {
				return false
			}
			return true
		}()
	}()

	go func() {
		condition <- func() bool {
			if len(exclude.Namespaces) == 0 {
				return true
			}
			if utils.ContainsNamepace(exclude.Namespaces, resource.GetNamespace()) {
				return false
			}
			return true
		}()
	}()

	go func() {
		condition <- func() bool {
			if exclude.Selector == nil {
				return true
			}
			selector, err := metav1.LabelSelectorAsSelector(exclude.Selector)
			// if the label selector is incorrect, should be fail or
			if err != nil {
				glog.Error(err)
				return false
			}
			if selector.Matches(labels.Set(resource.GetLabels())) {
				return false
			}
			return true
		}()
	}()

	go func() {
		condition <- func() bool {
			if len(exclude.Kinds) == 0 {
				return true
			}

			if findKind(exclude.Kinds, resource.GetKind()) {
				return false
			}

			return true
		}()
	}()

	var numberOfConditions = 9
	for numberOfConditions > 0 {
		select {
		case hasSucceeded := <-condition:
			if !hasSucceeded {
				return false
			}
		}
		numberOfConditions -= numberOfConditions
	}

	return true
}

//ParseNameFromObject extracts resource name from JSON obj
func ParseNameFromObject(bytes []byte) string {
	var objectJSON map[string]interface{}
	json.Unmarshal(bytes, &objectJSON)
	meta, ok := objectJSON["metadata"]
	if !ok {
		return ""
	}

	metaMap, ok := meta.(map[string]interface{})
	if !ok {
		return ""
	}
	if name, ok := metaMap["name"].(string); ok {
		return name
	}
	return ""
}

// ParseNamespaceFromObject extracts the namespace from the JSON obj
func ParseNamespaceFromObject(bytes []byte) string {
	var objectJSON map[string]interface{}
	json.Unmarshal(bytes, &objectJSON)
	meta, ok := objectJSON["metadata"]
	if !ok {
		return ""
	}
	metaMap, ok := meta.(map[string]interface{})
	if !ok {
		return ""
	}

	if name, ok := metaMap["namespace"].(string); ok {
		return name
	}

	return ""
}

func findKind(kinds []string, kindGVK string) bool {
	for _, kind := range kinds {
		if kind == kindGVK {
			return true
		}
	}
	return false
}

// validateGeneralRuleInfoVariables validate variable subtition defined in
// - MatchResources
// - ExcludeResources
// - Conditions
func validateGeneralRuleInfoVariables(ctx context.EvalInterface, rule kyverno.Rule) string {
	var tempRule kyverno.Rule
	var tempRulePattern interface{}

	tempRule.MatchResources = rule.MatchResources
	tempRule.ExcludeResources = rule.ExcludeResources
	tempRule.Conditions = rule.Conditions

	raw, err := json.Marshal(tempRule)
	if err != nil {
		glog.Infof("failed to serilize rule info while validating variable substitution: %v", err)
		return ""
	}

	if err := json.Unmarshal(raw, &tempRulePattern); err != nil {
		glog.Infof("failed to serilize rule info while validating variable substitution: %v", err)
		return ""
	}

	return variables.ValidateVariables(ctx, tempRulePattern)
}

func newPathNotPresentRuleResponse(rname, rtype, msg string) response.RuleResponse {
	return response.RuleResponse{
		Name:           rname,
		Type:           rtype,
		Message:        msg,
		Success:        true,
		PathNotPresent: true,
	}
}
