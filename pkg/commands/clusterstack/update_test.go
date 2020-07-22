// Copyright 2020-2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package clusterstack_test

import (
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	expv1alpha1 "github.com/pivotal/kpack/pkg/apis/experimental/v1alpha1"
	kpackfakes "github.com/pivotal/kpack/pkg/client/clientset/versioned/fake"
	"github.com/pivotal/kpack/pkg/registry/imagehelpers"
	"github.com/sclevine/spec"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgotesting "k8s.io/client-go/testing"

	stackpkg "github.com/pivotal/build-service-cli/pkg/clusterstack"
	"github.com/pivotal/build-service-cli/pkg/commands/clusterstack"
	"github.com/pivotal/build-service-cli/pkg/image/fakes"
	"github.com/pivotal/build-service-cli/pkg/testhelpers"
)

const expectedRepository = "some-registry.com/some-repo"

func TestUpdateCommand(t *testing.T) {
	spec.Run(t, "TestUpdateCommand", testUpdateCommand)
}

func testUpdateCommand(t *testing.T, when spec.G, it spec.S) {
	fetcher := &fakes.Fetcher{}

	oldBuildImage, oldBuildImageId, oldRunImage, oldRunImageId := makeStackImages(t, "some-old-id")
	fetcher.AddImage("some-old-build-image", oldBuildImage)
	fetcher.AddImage("some-old-run-image", oldRunImage)

	newBuildImage, newBuildImageId, newRunImage, newRunImageId := makeStackImages(t, "some-new-id")
	fetcher.AddImage("some-new-build-image", newBuildImage)
	fetcher.AddImage("some-new-run-image", newRunImage)

	relocator := &fakes.Relocator{}

	cmdFunc := func(clientSet *kpackfakes.Clientset) *cobra.Command {
		clientSetProvider := testhelpers.GetFakeKpackClusterProvider(clientSet)
		return clusterstack.NewUpdateCommand(clientSetProvider, fetcher, relocator)
	}

	stck := &expv1alpha1.ClusterStack{
		ObjectMeta: metav1.ObjectMeta{
			Name: "some-stack",
			Annotations: map[string]string{
				stackpkg.DefaultRepositoryAnnotation: expectedRepository,
			},
		},
		Spec: expv1alpha1.ClusterStackSpec{
			Id: "some-old-id",
			BuildImage: expv1alpha1.ClusterStackSpecImage{
				Image: "some-old-build-image",
			},
			RunImage: expv1alpha1.ClusterStackSpecImage{
				Image: "some-old-run-image",
			},
		},
		Status: expv1alpha1.ClusterStackStatus{
			ResolvedClusterStack: expv1alpha1.ResolvedClusterStack{
				Id: "some-old-id",
				BuildImage: expv1alpha1.ClusterStackStatusImage{
					LatestImage: "some-registry.com/old-repo/build@" + oldBuildImageId,
					Image:       "some-old-build-image",
				},
				RunImage: expv1alpha1.ClusterStackStatusImage{
					LatestImage: "some-registry.com/old-repo/run@" + oldRunImageId,
					Image:       "some-old-run-image",
				},
			},
		},
	}

	it("updates the stack id, run image, and build image", func() {
		testhelpers.CommandTest{
			Objects: []runtime.Object{
				stck,
			},
			Args:      []string{"some-stack", "--build-image", "some-new-build-image", "--run-image", "some-new-run-image"},
			ExpectErr: false,
			ExpectUpdates: []clientgotesting.UpdateActionImpl{
				{
					Object: &expv1alpha1.ClusterStack{
						ObjectMeta: stck.ObjectMeta,
						Spec: expv1alpha1.ClusterStackSpec{
							Id: "some-new-id",
							BuildImage: expv1alpha1.ClusterStackSpecImage{
								Image: "some-registry.com/some-repo/build@" + newBuildImageId,
							},
							RunImage: expv1alpha1.ClusterStackSpecImage{
								Image: "some-registry.com/some-repo/run@" + newRunImageId,
							},
						},
						Status: stck.Status,
					},
				},
			},
			ExpectedOutput: "Uploading to 'some-registry.com/some-repo'...\nClusterStack Updated\n",
		}.TestKpack(t, cmdFunc)
	})

	it("does not add stack images with the same digest", func() {
		testhelpers.CommandTest{
			Objects: []runtime.Object{
				stck,
			},
			Args:           []string{"some-stack", "--build-image", "some-old-build-image", "--run-image", "some-old-run-image"},
			ExpectErr:      false,
			ExpectedOutput: "Uploading to 'some-registry.com/some-repo'...\nBuild and Run images already exist in stack\nClusterStack Unchanged\n",
		}.TestKpack(t, cmdFunc)
	})

	it("returns error on invalid registry annotation", func() {
		stck.Annotations[stackpkg.DefaultRepositoryAnnotation] = ""

		testhelpers.CommandTest{
			Objects: []runtime.Object{
				stck,
			},
			Args:           []string{"some-stack", "--build-image", "some-new-build-image", "--run-image", "some-new-run-image"},
			ExpectErr:      true,
			ExpectedOutput: "Error: Unable to find default registry for clusterstack: some-stack\n",
		}.TestKpack(t, cmdFunc)
	})

	it("returns error when build image and run image have different stack Ids", func() {
		_, _, runImage, _ := makeStackImages(t, "other-stack-id")

		fetcher.AddImage("some-new-run-image", runImage)

		testhelpers.CommandTest{
			Objects: []runtime.Object{
				stck,
			},
			Args:           []string{"some-stack", "--build-image", "some-new-build-image", "--run-image", "some-new-run-image"},
			ExpectErr:      true,
			ExpectedOutput: "Uploading to 'some-registry.com/some-repo'...\nError: build stack 'some-new-id' does not match run stack 'other-stack-id'\n",
		}.TestKpack(t, cmdFunc)
	})
}

func makeStackImages(t *testing.T, stackId string) (v1.Image, string, v1.Image, string) {
	buildImage, err := random.Image(0, 0)
	if err != nil {
		t.Fatal(err)
	}

	buildImage, err = imagehelpers.SetStringLabel(buildImage, stackpkg.IdLabel, stackId)
	if err != nil {
		t.Fatal(err)
	}

	runImage, err := random.Image(0, 0)
	if err != nil {
		t.Fatal(err)
	}

	runImage, err = imagehelpers.SetStringLabel(runImage, stackpkg.IdLabel, stackId)
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
