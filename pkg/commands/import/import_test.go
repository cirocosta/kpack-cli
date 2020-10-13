package _import_test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/pivotal/kpack/pkg/apis/build/v1alpha1"
	kpackfakes "github.com/pivotal/kpack/pkg/client/clientset/versioned/fake"
	"github.com/pivotal/kpack/pkg/registry/imagehelpers"
	"github.com/sclevine/spec"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfakes "k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/pivotal/build-service-cli/pkg/clusterstack"
	"github.com/pivotal/build-service-cli/pkg/clusterstore"
	storefakes "github.com/pivotal/build-service-cli/pkg/clusterstore/fakes"
	commandsfakes "github.com/pivotal/build-service-cli/pkg/commands/fakes"
	importcmds "github.com/pivotal/build-service-cli/pkg/commands/import"
	"github.com/pivotal/build-service-cli/pkg/image/fakes"
	"github.com/pivotal/build-service-cli/pkg/testhelpers"
)

func TestImportCommand(t *testing.T) {
	spec.Run(t, "TestImportCommand", testImportCommand)
}

func testImportCommand(t *testing.T, when spec.G, it spec.S) {
	const (
		importTimestampKey = "kpack.io/import-timestamp"
	)
	fakeBuildpackageUploader := storefakes.FakeBuildpackageUploader{
		"some-registry.io/some-project/store-image":   "new-registry.io/new-project/store-image@sha256:123abc",
		"some-registry.io/some-project/store-image-2": "new-registry.io/new-project/store-image-2@sha256:456def",
	}

	storeFactory := &clusterstore.Factory{
		Uploader: fakeBuildpackageUploader,
	}

	buildImage, buildImageId, runImage, runImageId := makeStackImages(t, "some-stack-id")
	buildImage2, buildImage2Id, runImage2, runImage2Id := makeStackImages(t, "some-other-stack-id")

	fetcher := &fakes.Fetcher{}
	fetcher.AddImage("some-registry.io/some-project/build-image", buildImage)
	fetcher.AddImage("some-registry.io/some-project/run-image", runImage)
	fetcher.AddImage("some-registry.io/some-project/build-image-2", buildImage2)
	fetcher.AddImage("some-registry.io/some-project/run-image-2", runImage2)

	relocator := &fakes.Relocator{}

	stackFactory := &clusterstack.Factory{
		Fetcher:   fetcher,
		Relocator: relocator,
	}

	config := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kp-config",
			Namespace: "kpack",
		},
		Data: map[string]string{
			"canonical.repository":                "new-registry.io/new-project",
			"canonical.repository.serviceaccount": "some-serviceaccount",
		},
	}

	timestampProvider := FakeTimestampProvider{timestamp: "2006-01-02T15:04:05Z"}

	store := &v1alpha1.ClusterStore{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.ClusterStoreKind,
			APIVersion: "kpack.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "some-store",
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": `{"kind":"ClusterStore","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-store","creationTimestamp":null},"spec":{"sources":[{"image":"new-registry.io/new-project/store-image@sha256:123abc"}]},"status":{}}`,
				importTimestampKey: timestampProvider.timestamp,
			},
		},
		Spec: v1alpha1.ClusterStoreSpec{
			Sources: []v1alpha1.StoreImage{
				{Image: "new-registry.io/new-project/store-image@sha256:123abc"},
			},
		},
	}

	stack := &v1alpha1.ClusterStack{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.ClusterStackKind,
			APIVersion: "kpack.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "some-stack",
			Annotations: map[string]string{
				importTimestampKey: timestampProvider.timestamp,
			},
		},
		Spec: v1alpha1.ClusterStackSpec{
			Id: "some-stack-id",
			BuildImage: v1alpha1.ClusterStackSpecImage{
				Image: "new-registry.io/new-project/build@" + buildImageId,
			},
			RunImage: v1alpha1.ClusterStackSpecImage{
				Image: "new-registry.io/new-project/run@" + runImageId,
			},
		},
	}

	defaultStack := stack.DeepCopy()
	defaultStack.Name = "default"

	builder := &v1alpha1.ClusterBuilder{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.ClusterBuilderKind,
			APIVersion: "kpack.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "some-cb",
			Annotations: map[string]string{
				importTimestampKey: timestampProvider.timestamp,
			},
		},
		Spec: v1alpha1.ClusterBuilderSpec{
			BuilderSpec: v1alpha1.BuilderSpec{
				Tag: "new-registry.io/new-project/some-cb",
				Stack: corev1.ObjectReference{
					Name: "some-stack",
					Kind: v1alpha1.ClusterStackKind,
				},
				Store: corev1.ObjectReference{
					Name: "some-store",
					Kind: v1alpha1.ClusterStoreKind,
				},
				Order: []v1alpha1.OrderEntry{
					{
						Group: []v1alpha1.BuildpackRef{
							{
								BuildpackInfo: v1alpha1.BuildpackInfo{
									Id: "buildpack-1",
								},
							},
						},
					},
				},
			},
			ServiceAccountRef: corev1.ObjectReference{
				Namespace: "kpack",
				Name:      "some-serviceaccount",
			},
		},
	}

	defaultBuilder := builder.DeepCopy()
	defaultBuilder.Name = "default"
	defaultBuilder.Spec.Tag = "new-registry.io/new-project/default"

	var fakeConfirmationProvider *commandsfakes.FakeConfirmationProvider

	cmdFunc := func(k8sClientSet *k8sfakes.Clientset, kpackClientSet *kpackfakes.Clientset) *cobra.Command {
		clientSetProvider := testhelpers.GetFakeClusterProvider(k8sClientSet, kpackClientSet)
		return importcmds.NewImportCommand(clientSetProvider, timestampProvider, storeFactory, stackFactory, fakeConfirmationProvider)
	}

	it.Before(func() {
		fakeConfirmationProvider = commandsfakes.NewFakeConfirmationProvider(true, nil)
	})

	when("there are no stores, stacks, or cbs", func() {
		it("creates stores, stacks, and cbs defined in the dependency descriptor", func() {
			builder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`
			defaultBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`

			testhelpers.CommandTest{
				K8sObjects: []runtime.Object{
					config,
				},
				Args: []string{
					"-f", "./testdata/deps.yaml",
					"--registry-ca-cert-path", "some-cert-path",
					"--registry-verify-certs",
				},
				ExpectedOutput: `Importing ClusterStore 'some-store'...
Importing ClusterStack 'some-stack'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterStack 'default'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterBuilder 'some-cb'...
Importing ClusterBuilder 'default'...
Imported resources created
`,
				ExpectCreates: []runtime.Object{
					store,
					stack,
					defaultStack,
					builder,
					defaultBuilder,
				},
			}.TestK8sAndKpack(t, cmdFunc)
			require.Equal(t, true, fakeConfirmationProvider.WasRequested())
		})

		it("creates stores, stacks, and cbs defined in the dependency descriptor for version 1", func() {
			builder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`
			defaultBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`

			testhelpers.CommandTest{
				K8sObjects: []runtime.Object{
					config,
				},
				Args: []string{
					"-f", "./testdata/v1-deps.yaml",
					"--registry-ca-cert-path", "some-cert-path",
					"--registry-verify-certs",
				},
				ExpectedOutput: `Importing ClusterStore 'some-store'...
Importing ClusterStack 'some-stack'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterStack 'default'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterBuilder 'some-cb'...
Importing ClusterBuilder 'default'...
Imported resources created
`,
				ExpectCreates: []runtime.Object{
					store,
					stack,
					defaultStack,
					builder,
					defaultBuilder,
				},
			}.TestK8sAndKpack(t, cmdFunc)
		})

		it("skips confirmation when the force flag is used", func(){
			builder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`
			defaultBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`

			testhelpers.CommandTest{
				K8sObjects: []runtime.Object{
					config,
				},
				Args: []string{
					"-f", "./testdata/deps.yaml",
					"--registry-ca-cert-path", "some-cert-path",
					"--registry-verify-certs",
					"--force",
				},
				ExpectedOutput: `Importing Cluster Store 'some-store'...
Importing Cluster Stack 'some-stack'...
Uploading to 'new-registry.io/new-project'...
Importing Cluster Stack 'default'...
Uploading to 'new-registry.io/new-project'...
Importing Cluster Builder 'some-cb'...
Importing Cluster Builder 'default'...
Imported resources created
`,
				ExpectCreates: []runtime.Object{
					store,
					stack,
					defaultStack,
					builder,
					defaultBuilder,
				},
			}.TestK8sAndKpack(t, cmdFunc)
			require.Equal(t, false, fakeConfirmationProvider.WasRequested())
		})
	})

	when("there are existing stores, stacks, or cbs", func() {
		when("the dependency descriptor and the store have the exact same objects", func() {
			const newTimestamp = "new-timestamp"
			timestampProvider.timestamp = newTimestamp

			expectedStore := store.DeepCopy()
			expectedStore.Annotations[importTimestampKey] = newTimestamp

			expectedStack := stack.DeepCopy()
			expectedStack.Annotations[importTimestampKey] = newTimestamp

			expectedDefaultStack := defaultStack.DeepCopy()
			expectedDefaultStack.Annotations[importTimestampKey] = newTimestamp

			expectedBuilder := builder.DeepCopy()
			expectedBuilder.Annotations[importTimestampKey] = newTimestamp

			expectedDefaultBuilder := defaultBuilder.DeepCopy()
			expectedDefaultBuilder.Annotations[importTimestampKey] = newTimestamp

			it("updates the import timestamp", func() {
				expectedBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`
				expectedDefaultBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`

				stack.Spec.BuildImage.Image = fmt.Sprintf("new-registry.io/new-project/build@%s", buildImageId)
				stack.Spec.RunImage.Image = fmt.Sprintf("new-registry.io/new-project/run@%s", runImageId)

				defaultStack.Spec.BuildImage.Image = fmt.Sprintf("new-registry.io/new-project/build@%s", buildImageId)
				defaultStack.Spec.RunImage.Image = fmt.Sprintf("new-registry.io/new-project/run@%s", runImageId)

				testhelpers.CommandTest{
					K8sObjects: []runtime.Object{
						config,
					},
					KpackObjects: []runtime.Object{
						store,
						stack,
						defaultStack,
						builder,
						defaultBuilder,
					},
					Args: []string{
						"-f", "./testdata/deps.yaml",
					},
					ExpectedOutput: `Importing ClusterStore 'some-store'...
	Buildpackage already exists in the store
Importing ClusterStack 'some-stack'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterStack 'default'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterBuilder 'some-cb'...
Importing ClusterBuilder 'default'...
Imported resources created
`,
					ExpectUpdates: []clientgotesting.UpdateActionImpl{
						{
							Object: expectedStore,
						},
						{
							Object: expectedStack,
						},
						{
							Object: expectedDefaultStack,
						},
						{
							Object: expectedBuilder,
						},
						{
							Object: expectedDefaultBuilder,
						},
					},
				}.TestK8sAndKpack(t, cmdFunc)
			})

			it("does not error when original resource annotation is nil", func() {
				store.Annotations = nil
				stack.Annotations = nil
				defaultStack.Annotations = nil
				builder.Annotations = nil
				defaultBuilder.Annotations = nil

				expectedStore.Annotations = map[string]string{importTimestampKey: newTimestamp}
				expectedBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`
				expectedDefaultBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`

				testhelpers.CommandTest{
					K8sObjects: []runtime.Object{
						config,
					},
					KpackObjects: []runtime.Object{
						store,
						stack,
						defaultStack,
						builder,
						defaultBuilder,
					},
					Args: []string{
						"-f", "./testdata/deps.yaml",
					},
					ExpectedOutput: `Importing ClusterStore 'some-store'...
	Buildpackage already exists in the store
Importing ClusterStack 'some-stack'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterStack 'default'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterBuilder 'some-cb'...
Importing ClusterBuilder 'default'...
Imported resources created
`,
					ExpectUpdates: []clientgotesting.UpdateActionImpl{
						{
							Object: expectedStore,
						},
						{
							Object: expectedStack,
						},
						{
							Object: expectedDefaultStack,
						},
						{
							Object: expectedBuilder,
						},
						{
							Object: expectedDefaultBuilder,
						},
					},
				}.TestK8sAndKpack(t, cmdFunc)
			})
		})

		when("the dependency descriptor has different resources", func() {
			const newTimestamp = "new-timestamp"
			timestampProvider.timestamp = newTimestamp

			expectedStore := store.DeepCopy()
			expectedStore.Annotations[importTimestampKey] = newTimestamp
			expectedStore.Spec.Sources = append(expectedStore.Spec.Sources, v1alpha1.StoreImage{
				Image: "new-registry.io/new-project/store-image-2@sha256:456def",
			})

			expectedStack := stack.DeepCopy()
			expectedStack.Annotations[importTimestampKey] = newTimestamp
			expectedStack.Spec.Id = "some-other-stack-id"
			expectedStack.Spec.BuildImage.Image = fmt.Sprintf("new-registry.io/new-project/build@%s", buildImage2Id)
			expectedStack.Spec.RunImage.Image = fmt.Sprintf("new-registry.io/new-project/run@%s", runImage2Id)

			expectedDefaultStack := defaultStack.DeepCopy()
			expectedDefaultStack.Annotations[importTimestampKey] = newTimestamp
			expectedDefaultStack.Spec.Id = "some-other-stack-id"
			expectedDefaultStack.Spec.BuildImage.Image = fmt.Sprintf("new-registry.io/new-project/build@%s", buildImage2Id)
			expectedDefaultStack.Spec.RunImage.Image = fmt.Sprintf("new-registry.io/new-project/run@%s", runImage2Id)

			expectedBuilder := builder.DeepCopy()
			expectedBuilder.Annotations[importTimestampKey] = newTimestamp
			expectedBuilder.Spec.Order = []v1alpha1.OrderEntry{
				{
					Group: []v1alpha1.BuildpackRef{
						{
							BuildpackInfo: v1alpha1.BuildpackInfo{
								Id: "buildpack-2",
							},
						},
					},
				},
			}

			expectedDefaultBuilder := defaultBuilder.DeepCopy()
			expectedDefaultBuilder.Annotations[importTimestampKey] = newTimestamp
			expectedDefaultBuilder.Spec.Order = []v1alpha1.OrderEntry{
				{
					Group: []v1alpha1.BuildpackRef{
						{
							BuildpackInfo: v1alpha1.BuildpackInfo{
								Id: "buildpack-2",
							},
						},
					},
				},
			}

			expectedBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-2"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`
			expectedDefaultBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-2"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`

			it("creates stores, stacks, and cbs defined in the dependency descriptor and updates the timestamp", func() {
				testhelpers.CommandTest{
					K8sObjects: []runtime.Object{
						config,
					},
					KpackObjects: []runtime.Object{
						store,
						stack,
						defaultStack,
						builder,
						defaultBuilder,
					},
					Args: []string{
						"-f", "./testdata/updated-deps.yaml",
					},
					ExpectedOutput: `Importing ClusterStore 'some-store'...
	Added Buildpackage
Importing ClusterStack 'some-stack'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterStack 'default'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterBuilder 'some-cb'...
Importing ClusterBuilder 'default'...
Imported resources created
`,
					ExpectUpdates: []clientgotesting.UpdateActionImpl{
						{
							Object: expectedStore,
						},
						{
							Object: expectedStack,
						},
						{
							Object: expectedDefaultStack,
						},
						{
							Object: expectedBuilder,
						},
						{
							Object: expectedDefaultBuilder,
						},
					},
				}.TestK8sAndKpack(t, cmdFunc)
			})
		})
	})

	it("errors when the apiVersion is unexpected", func() {
		testhelpers.CommandTest{
			K8sObjects: []runtime.Object{},
			Args: []string{
				"-f", "./testdata/invalid-deps.yaml",
			},
			ExpectedOutput: "Error: did not find expected apiVersion, must be one of: [kp.kpack.io/v1alpha1 kp.kpack.io/v1alpha2]\n",
			ExpectErr:      true,
		}.TestK8sAndKpack(t, cmdFunc)
	})

	when("output flag is used", func() {
		const expectedOutput = `Importing ClusterStore 'some-store'...
Importing ClusterStack 'some-stack'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterStack 'default'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterBuilder 'some-cb'...
Importing ClusterBuilder 'default'...
`

		builder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`
		defaultBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`

		it("can output in yaml format", func() {
			const resourceYAML = `apiVersion: kpack.io/v1alpha1
kind: ClusterStore
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
    kubectl.kubernetes.io/last-applied-configuration: '{"kind":"ClusterStore","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-store","creationTimestamp":null},"spec":{"sources":[{"image":"new-registry.io/new-project/store-image@sha256:123abc"}]},"status":{}}'
  creationTimestamp: null
  name: some-store
spec:
  sources:
  - image: new-registry.io/new-project/store-image@sha256:123abc
status: {}
---
apiVersion: kpack.io/v1alpha1
kind: ClusterStack
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
  creationTimestamp: null
  name: some-stack
spec:
  buildImage:
    image: new-registry.io/new-project/build@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d
  id: some-stack-id
  runImage:
    image: new-registry.io/new-project/run@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d
status:
  buildImage: {}
  runImage: {}
---
apiVersion: kpack.io/v1alpha1
kind: ClusterStack
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
  creationTimestamp: null
  name: default
spec:
  buildImage:
    image: new-registry.io/new-project/build@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d
  id: some-stack-id
  runImage:
    image: new-registry.io/new-project/run@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d
status:
  buildImage: {}
  runImage: {}
---
apiVersion: kpack.io/v1alpha1
kind: ClusterBuilder
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
    kubectl.kubernetes.io/last-applied-configuration: '{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}'
  creationTimestamp: null
  name: some-cb
spec:
  order:
  - group:
    - id: buildpack-1
  serviceAccountRef:
    name: some-serviceaccount
    namespace: kpack
  stack:
    kind: ClusterStack
    name: some-stack
  store:
    kind: ClusterStore
    name: some-store
  tag: new-registry.io/new-project/some-cb
status:
  stack: {}
---
apiVersion: kpack.io/v1alpha1
kind: ClusterBuilder
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
    kubectl.kubernetes.io/last-applied-configuration: '{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}'
  creationTimestamp: null
  name: default
spec:
  order:
  - group:
    - id: buildpack-1
  serviceAccountRef:
    name: some-serviceaccount
    namespace: kpack
  stack:
    kind: ClusterStack
    name: some-stack
  store:
    kind: ClusterStore
    name: some-store
  tag: new-registry.io/new-project/default
status:
  stack: {}
`

			testhelpers.CommandTest{
				K8sObjects: []runtime.Object{
					config,
				},
				Args: []string{
					"-f", "./testdata/deps.yaml",
					"--output", "yaml",
				},
				ExpectedOutput:      resourceYAML,
				ExpectedErrorOutput: expectedOutput,
				ExpectCreates: []runtime.Object{
					store,
					stack,
					defaultStack,
					builder,
					defaultBuilder,
				},
			}.TestK8sAndKpack(t, cmdFunc)
		})

		it("can output in json format", func() {
			const resourceJSON = `{
    "kind": "ClusterStore",
    "apiVersion": "kpack.io/v1alpha1",
    "metadata": {
        "name": "some-store",
        "creationTimestamp": null,
        "annotations": {
            "kpack.io/import-timestamp": "2006-01-02T15:04:05Z",
            "kubectl.kubernetes.io/last-applied-configuration": "{\"kind\":\"ClusterStore\",\"apiVersion\":\"kpack.io/v1alpha1\",\"metadata\":{\"name\":\"some-store\",\"creationTimestamp\":null},\"spec\":{\"sources\":[{\"image\":\"new-registry.io/new-project/store-image@sha256:123abc\"}]},\"status\":{}}"
        }
    },
    "spec": {
        "sources": [
            {
                "image": "new-registry.io/new-project/store-image@sha256:123abc"
            }
        ]
    },
    "status": {}
}
{
    "kind": "ClusterStack",
    "apiVersion": "kpack.io/v1alpha1",
    "metadata": {
        "name": "some-stack",
        "creationTimestamp": null,
        "annotations": {
            "kpack.io/import-timestamp": "2006-01-02T15:04:05Z"
        }
    },
    "spec": {
        "id": "some-stack-id",
        "buildImage": {
            "image": "new-registry.io/new-project/build@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d"
        },
        "runImage": {
            "image": "new-registry.io/new-project/run@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d"
        }
    },
    "status": {
        "buildImage": {},
        "runImage": {}
    }
}
{
    "kind": "ClusterStack",
    "apiVersion": "kpack.io/v1alpha1",
    "metadata": {
        "name": "default",
        "creationTimestamp": null,
        "annotations": {
            "kpack.io/import-timestamp": "2006-01-02T15:04:05Z"
        }
    },
    "spec": {
        "id": "some-stack-id",
        "buildImage": {
            "image": "new-registry.io/new-project/build@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d"
        },
        "runImage": {
            "image": "new-registry.io/new-project/run@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d"
        }
    },
    "status": {
        "buildImage": {},
        "runImage": {}
    }
}
{
    "kind": "ClusterBuilder",
    "apiVersion": "kpack.io/v1alpha1",
    "metadata": {
        "name": "some-cb",
        "creationTimestamp": null,
        "annotations": {
            "kpack.io/import-timestamp": "2006-01-02T15:04:05Z",
            "kubectl.kubernetes.io/last-applied-configuration": "{\"kind\":\"ClusterBuilder\",\"apiVersion\":\"kpack.io/v1alpha1\",\"metadata\":{\"name\":\"some-cb\",\"creationTimestamp\":null},\"spec\":{\"tag\":\"new-registry.io/new-project/some-cb\",\"stack\":{\"kind\":\"ClusterStack\",\"name\":\"some-stack\"},\"store\":{\"kind\":\"ClusterStore\",\"name\":\"some-store\"},\"order\":[{\"group\":[{\"id\":\"buildpack-1\"}]}],\"serviceAccountRef\":{\"namespace\":\"kpack\",\"name\":\"some-serviceaccount\"}},\"status\":{\"stack\":{}}}"
        }
    },
    "spec": {
        "tag": "new-registry.io/new-project/some-cb",
        "stack": {
            "kind": "ClusterStack",
            "name": "some-stack"
        },
        "store": {
            "kind": "ClusterStore",
            "name": "some-store"
        },
        "order": [
            {
                "group": [
                    {
                        "id": "buildpack-1"
                    }
                ]
            }
        ],
        "serviceAccountRef": {
            "namespace": "kpack",
            "name": "some-serviceaccount"
        }
    },
    "status": {
        "stack": {}
    }
}
{
    "kind": "ClusterBuilder",
    "apiVersion": "kpack.io/v1alpha1",
    "metadata": {
        "name": "default",
        "creationTimestamp": null,
        "annotations": {
            "kpack.io/import-timestamp": "2006-01-02T15:04:05Z",
            "kubectl.kubernetes.io/last-applied-configuration": "{\"kind\":\"ClusterBuilder\",\"apiVersion\":\"kpack.io/v1alpha1\",\"metadata\":{\"name\":\"default\",\"creationTimestamp\":null},\"spec\":{\"tag\":\"new-registry.io/new-project/default\",\"stack\":{\"kind\":\"ClusterStack\",\"name\":\"some-stack\"},\"store\":{\"kind\":\"ClusterStore\",\"name\":\"some-store\"},\"order\":[{\"group\":[{\"id\":\"buildpack-1\"}]}],\"serviceAccountRef\":{\"namespace\":\"kpack\",\"name\":\"some-serviceaccount\"}},\"status\":{\"stack\":{}}}"
        }
    },
    "spec": {
        "tag": "new-registry.io/new-project/default",
        "stack": {
            "kind": "ClusterStack",
            "name": "some-stack"
        },
        "store": {
            "kind": "ClusterStore",
            "name": "some-store"
        },
        "order": [
            {
                "group": [
                    {
                        "id": "buildpack-1"
                    }
                ]
            }
        ],
        "serviceAccountRef": {
            "namespace": "kpack",
            "name": "some-serviceaccount"
        }
    },
    "status": {
        "stack": {}
    }
}
`

			testhelpers.CommandTest{
				K8sObjects: []runtime.Object{
					config,
				},
				Args: []string{
					"-f", "./testdata/deps.yaml",
					"--output", "json",
				},
				ExpectedOutput:      resourceJSON,
				ExpectedErrorOutput: expectedOutput,
				ExpectCreates: []runtime.Object{
					store,
					stack,
					defaultStack,
					builder,
					defaultBuilder,
				},
			}.TestK8sAndKpack(t, cmdFunc)
		})
	})

	when("dry-run flag is used", func() {
		builder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`
		defaultBuilder.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = `{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}`

		it("does not create any resources and prints result with dry run indicated", func() {
			const expectedOutput = `Importing ClusterStore 'some-store'...
Importing ClusterStack 'some-stack'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterStack 'default'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterBuilder 'some-cb'...
Importing ClusterBuilder 'default'...
Imported resources created (dry run)
`

			testhelpers.CommandTest{
				K8sObjects: []runtime.Object{
					config,
				},
				Args: []string{
					"-f", "./testdata/deps.yaml",
					"--dry-run",
				},
				ExpectedOutput: expectedOutput,
			}.TestK8sAndKpack(t, cmdFunc)
		})

		when("output flag is used", func() {
			const resourceYAML = `apiVersion: kpack.io/v1alpha1
kind: ClusterStore
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
    kubectl.kubernetes.io/last-applied-configuration: '{"kind":"ClusterStore","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-store","creationTimestamp":null},"spec":{"sources":[{"image":"new-registry.io/new-project/store-image@sha256:123abc"}]},"status":{}}'
  creationTimestamp: null
  name: some-store
spec:
  sources:
  - image: new-registry.io/new-project/store-image@sha256:123abc
status: {}
---
apiVersion: kpack.io/v1alpha1
kind: ClusterStack
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
  creationTimestamp: null
  name: some-stack
spec:
  buildImage:
    image: new-registry.io/new-project/build@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d
  id: some-stack-id
  runImage:
    image: new-registry.io/new-project/run@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d
status:
  buildImage: {}
  runImage: {}
---
apiVersion: kpack.io/v1alpha1
kind: ClusterStack
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
  creationTimestamp: null
  name: default
spec:
  buildImage:
    image: new-registry.io/new-project/build@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d
  id: some-stack-id
  runImage:
    image: new-registry.io/new-project/run@sha256:9dc5608d0f7f31ecd4cd26c00ec56629180dc29bba3423e26fc87317e1c2846d
status:
  buildImage: {}
  runImage: {}
---
apiVersion: kpack.io/v1alpha1
kind: ClusterBuilder
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
    kubectl.kubernetes.io/last-applied-configuration: '{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"some-cb","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/some-cb","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}'
  creationTimestamp: null
  name: some-cb
spec:
  order:
  - group:
    - id: buildpack-1
  serviceAccountRef:
    name: some-serviceaccount
    namespace: kpack
  stack:
    kind: ClusterStack
    name: some-stack
  store:
    kind: ClusterStore
    name: some-store
  tag: new-registry.io/new-project/some-cb
status:
  stack: {}
---
apiVersion: kpack.io/v1alpha1
kind: ClusterBuilder
metadata:
  annotations:
    kpack.io/import-timestamp: "2006-01-02T15:04:05Z"
    kubectl.kubernetes.io/last-applied-configuration: '{"kind":"ClusterBuilder","apiVersion":"kpack.io/v1alpha1","metadata":{"name":"default","creationTimestamp":null},"spec":{"tag":"new-registry.io/new-project/default","stack":{"kind":"ClusterStack","name":"some-stack"},"store":{"kind":"ClusterStore","name":"some-store"},"order":[{"group":[{"id":"buildpack-1"}]}],"serviceAccountRef":{"namespace":"kpack","name":"some-serviceaccount"}},"status":{"stack":{}}}'
  creationTimestamp: null
  name: default
spec:
  order:
  - group:
    - id: buildpack-1
  serviceAccountRef:
    name: some-serviceaccount
    namespace: kpack
  stack:
    kind: ClusterStack
    name: some-stack
  store:
    kind: ClusterStore
    name: some-store
  tag: new-registry.io/new-project/default
status:
  stack: {}
`

			const expectedOutput = `Importing ClusterStore 'some-store'...
Importing ClusterStack 'some-stack'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterStack 'default'...
Uploading to 'new-registry.io/new-project'...
Importing ClusterBuilder 'some-cb'...
Importing ClusterBuilder 'default'...
`

			it("does not create a Builder and prints the resource output", func() {
				testhelpers.CommandTest{
					K8sObjects: []runtime.Object{
						config,
					},
					Args: []string{
						"-f", "./testdata/deps.yaml",
						"--dry-run",
						"--output", "yaml",
					},
					ExpectedOutput:      resourceYAML,
					ExpectedErrorOutput: expectedOutput,
				}.TestK8sAndKpack(t, cmdFunc)
			})
		})
	})
}

func makeStackImages(t *testing.T, stackId string) (v1.Image, string, v1.Image, string) {
	buildImage, err := random.Image(0, 0)
	if err != nil {
		t.Fatal(err)
	}

	buildImage, err = imagehelpers.SetStringLabel(buildImage, clusterstack.IdLabel, stackId)
	if err != nil {
		t.Fatal(err)
	}

	runImage, err := random.Image(0, 0)
	if err != nil {
		t.Fatal(err)
	}

	runImage, err = imagehelpers.SetStringLabel(runImage, clusterstack.IdLabel, stackId)
	if err != nil {
		t.Fatal(err)
	}

	buildImageHash, err := buildImage.Digest()
	if err != nil {
		t.Fatal(err)
	}

	runImageHash, err := runImage.Digest()
	if err != nil {
		t.Fatal(err)
	}

	return buildImage, buildImageHash.String(), runImage, runImageHash.String()
}

type FakeTimestampProvider struct {
	timestamp string
}

func (f FakeTimestampProvider) GetTimestamp() string {
	return f.timestamp
}
