package custombuilder_test

import (
	"testing"

	expv1alpha1 "github.com/pivotal/kpack/pkg/apis/experimental/v1alpha1"
	"github.com/pivotal/kpack/pkg/client/clientset/versioned/fake"
	"github.com/sclevine/spec"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/pivotal/build-service-cli/pkg/commands/custombuilder"
	"github.com/pivotal/build-service-cli/pkg/testhelpers"
)

func TestCustomBuilderCreateCommand(t *testing.T) {
	spec.Run(t, "TestCustomBuilderCreateCommand", testCustomBuilderCreateCommand)
}

func testCustomBuilderCreateCommand(t *testing.T, when spec.G, it spec.S) {
	const defaultNamespace = "some-default-namespace"

	var (
		expectedBuilder = &expv1alpha1.CustomBuilder{
			TypeMeta: metav1.TypeMeta{
				Kind:       expv1alpha1.CustomBuilderKind,
				APIVersion: "experimental.kpack.pivotal.io/v1alpha1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-builder",
				Namespace: "some-namespace",
				Annotations: map[string]string{
					"kubectl.kubernetes.io/last-applied-configuration": `{"kind":"CustomBuilder","apiVersion":"experimental.kpack.pivotal.io/v1alpha1","metadata":{"name":"test-builder","namespace":"some-namespace","creationTimestamp":null},"spec":{"tag":"some-registry.com/test-builder","stack":"some-stack","store":"some-store","order":[{"group":[{"id":"org.cloudfoundry.nodejs"}]},{"group":[{"id":"org.cloudfoundry.go"}]}],"serviceAccount":"default"},"status":{"stack":{}}}`,
				},
			},
			Spec: expv1alpha1.CustomNamespacedBuilderSpec{
				CustomBuilderSpec: expv1alpha1.CustomBuilderSpec{
					Tag:   "some-registry.com/test-builder",
					Stack: "some-stack",
					Store: "some-store",
					Order: []expv1alpha1.OrderEntry{
						{
							Group: []expv1alpha1.BuildpackRef{
								{
									BuildpackInfo: expv1alpha1.BuildpackInfo{
										Id: "org.cloudfoundry.nodejs",
									},
								},
							},
						},
						{
							Group: []expv1alpha1.BuildpackRef{
								{
									BuildpackInfo: expv1alpha1.BuildpackInfo{
										Id: "org.cloudfoundry.go",
									},
								},
							},
						},
					},
				},
				ServiceAccount: "default",
			},
		}
	)

	cmdFunc := func(clientSet *fake.Clientset) *cobra.Command {
		clientSetProvider := testhelpers.GetFakeKpackProvider(clientSet, defaultNamespace)
		return custombuilder.NewCreateCommand(clientSetProvider)
	}

	it("creates a CustomBuilder", func() {
		testhelpers.CommandTest{
			Args: []string{
				expectedBuilder.Name,
				expectedBuilder.Spec.Tag,
				"--stack", expectedBuilder.Spec.Stack,
				"--store", expectedBuilder.Spec.Store,
				"--order", "./testdata/order.yaml",
				"-n", expectedBuilder.Namespace,
			},
			ExpectedOutput: `"test-builder" created
`,
			ExpectCreates: []runtime.Object{
				expectedBuilder,
			},
		}.TestKpack(t, cmdFunc)
	})

	it("creates a CustomBuilder with the default namespace, store, and stack", func() {
		expectedBuilder.Namespace = defaultNamespace
		expectedBuilder.Spec.Stack = "default"
		expectedBuilder.Spec.Store = "default"
		expectedBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"CustomBuilder","apiVersion":"experimental.kpack.pivotal.io/v1alpha1","metadata":{"name":"test-builder","namespace":"some-default-namespace","creationTimestamp":null},"spec":{"tag":"some-registry.com/test-builder","stack":"default","store":"default","order":[{"group":[{"id":"org.cloudfoundry.nodejs"}]},{"group":[{"id":"org.cloudfoundry.go"}]}],"serviceAccount":"default"},"status":{"stack":{}}}`

		testhelpers.CommandTest{
			Args: []string{
				expectedBuilder.Name,
				expectedBuilder.Spec.Tag,
				"--order", "./testdata/order.yaml",
			},
			ExpectedOutput: "\"test-builder\" created\n",
			ExpectCreates: []runtime.Object{
				expectedBuilder,
			},
		}.TestKpack(t, cmdFunc)
	})

}
