package freight

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
)

func TestNewWebhook(t *testing.T) {
	kubeClient := fake.NewClientBuilder().Build()
	w := newWebhook(kubeClient)
	// Assert that all overridable behaviors were initialized to a default:
	require.NotNil(t, w.validateProjectFn)
	require.NotNil(t, w.listFreightFn)
}

func TestDefault(t *testing.T) {
	freight := &kargoapi.Freight{
		Commits: []kargoapi.GitCommit{
			{
				RepoURL: "fake-repo-url",
				ID:      "fake-id",
			},
		},
	}
	w := &webhook{}
	err := w.Default(context.Background(), freight)
	require.NoError(t, err)
	require.NotEmpty(t, freight.ID)
	require.NotEmpty(t, freight.Name)
	require.Equal(t, freight.ID, freight.Name)
}

func TestValidateCreate(t *testing.T) {
	testCases := []struct {
		name       string
		webhook    *webhook
		freight    kargoapi.Freight
		assertions func(error)
	}{
		{
			name: "error validating project",
			webhook: &webhook{
				validateProjectFn: func(
					context.Context,
					client.Client,
					schema.GroupKind,
					client.Object,
				) error {
					return errors.New("something went wrong")
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Equal(t, "something went wrong", err.Error())
			},
		},
		{
			name: "error listing freight",
			freight: kargoapi.Freight{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kargoapi.AliasLabelKey: "fake-alias",
					},
				},
			},
			webhook: &webhook{
				validateProjectFn: func(
					context.Context,
					client.Client,
					schema.GroupKind,
					client.Object,
				) error {
					return nil
				},
				listFreightFn: func(
					context.Context,
					client.ObjectList,
					...client.ListOption,
				) error {
					return errors.New("something went wrong")
				},
			},
			assertions: func(err error) {
				statusErr, ok := err.(*apierrors.StatusError)
				require.True(t, ok)
				require.Equal(
					t,
					int32(http.StatusInternalServerError),
					statusErr.Status().Code,
				)
			},
		},
		{
			name: "alias already in use",
			freight: kargoapi.Freight{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kargoapi.AliasLabelKey: "fake-alias",
					},
				},
			},
			webhook: &webhook{
				validateProjectFn: func(
					context.Context,
					client.Client,
					schema.GroupKind,
					client.Object,
				) error {
					return nil
				},
				listFreightFn: func(
					_ context.Context,
					objList client.ObjectList,
					_ ...client.ListOption,
				) error {
					freight, ok := objList.(*kargoapi.FreightList)
					require.True(t, ok)
					freight.Items = []kargoapi.Freight{{}}
					return nil
				},
			},
			assertions: func(err error) {
				statusErr, ok := err.(*apierrors.StatusError)
				require.True(t, ok)
				require.Equal(t, int32(http.StatusConflict), statusErr.Status().Code)
			},
		},
		{
			name: "no artifacts",
			webhook: &webhook{
				validateProjectFn: func(
					context.Context,
					client.Client,
					schema.GroupKind,
					client.Object,
				) error {
					return nil
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(
					t,
					err.Error(),
					"freight must contain at least one commit, image, or chart",
				)
			},
		},
		{
			name: "success",
			webhook: &webhook{
				validateProjectFn: func(
					context.Context,
					client.Client,
					schema.GroupKind,
					client.Object,
				) error {
					return nil
				},
			},
			freight: kargoapi.Freight{
				Commits: []kargoapi.GitCommit{{}},
			},
			assertions: func(err error) {
				require.NoError(t, err)
			},
		},
	}
	for _, testCase := range testCases {
		tc := testCase // Avoid implicit memory aliasing
		t.Run(testCase.name, func(t *testing.T) {
			tc.assertions(
				tc.webhook.ValidateCreate(context.Background(), &tc.freight),
			)
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	testCases := []struct {
		name       string
		webhook    *webhook
		setup      func() (*kargoapi.Freight, *kargoapi.Freight)
		assertions func(error)
	}{
		{
			name: "error listing freight",
			setup: func() (*kargoapi.Freight, *kargoapi.Freight) {
				return &kargoapi.Freight{}, &kargoapi.Freight{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kargoapi.AliasLabelKey: "fake-alias",
						},
					},
				}
			},
			webhook: &webhook{
				validateProjectFn: func(
					context.Context,
					client.Client,
					schema.GroupKind,
					client.Object,
				) error {
					return nil
				},
				listFreightFn: func(
					context.Context,
					client.ObjectList,
					...client.ListOption,
				) error {
					return errors.New("something went wrong")
				},
			},
			assertions: func(err error) {
				statusErr, ok := err.(*apierrors.StatusError)
				require.True(t, ok)
				require.Equal(
					t,
					int32(http.StatusInternalServerError),
					statusErr.Status().Code,
				)
			},
		},
		{
			name: "alias already in use",
			setup: func() (*kargoapi.Freight, *kargoapi.Freight) {
				return &kargoapi.Freight{}, &kargoapi.Freight{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kargoapi.AliasLabelKey: "fake-alias",
						},
					},
				}
			},
			webhook: &webhook{
				validateProjectFn: func(
					context.Context,
					client.Client,
					schema.GroupKind,
					client.Object,
				) error {
					return nil
				},
				listFreightFn: func(
					_ context.Context,
					objList client.ObjectList,
					_ ...client.ListOption,
				) error {
					freight, ok := objList.(*kargoapi.FreightList)
					require.True(t, ok)
					freight.Items = []kargoapi.Freight{{}, {}}
					return nil
				},
			},
			assertions: func(err error) {
				statusErr, ok := err.(*apierrors.StatusError)
				require.True(t, ok)
				require.Equal(t, int32(http.StatusConflict), statusErr.Status().Code)
			},
		},

		{
			name: "attempt to mutate",
			setup: func() (*kargoapi.Freight, *kargoapi.Freight) {
				oldFreight := &kargoapi.Freight{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fake-name",
						Namespace: "fake-namespace",
					},
					ID: "fake-id",
				}
				newFreight := oldFreight.DeepCopy()
				newFreight.ID = "another-fake-id"
				return oldFreight, newFreight
			},
			webhook: &webhook{},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "\"fake-name\" is invalid")
				require.Contains(t, err.Error(), "freight is immutable")
			},
		},

		{
			name: "update without mutation",
			setup: func() (*kargoapi.Freight, *kargoapi.Freight) {
				oldFreight := &kargoapi.Freight{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fake-name",
						Namespace: "fake-namespace",
					},
					ID: "fake-id",
				}
				newFreight := oldFreight.DeepCopy()
				return oldFreight, newFreight
			},
			webhook: &webhook{},
			assertions: func(err error) {
				require.NoError(t, err)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			oldFreight, newFreight := testCase.setup()
			testCase.assertions(
				testCase.webhook.ValidateUpdate(
					context.Background(),
					oldFreight,
					newFreight,
				),
			)
		})
	}
}

func TestValidateDelete(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, kargoapi.AddToScheme(scheme))

	testCases := map[string]struct {
		clientBuilderFunc func(*fake.ClientBuilder) *fake.ClientBuilder
		input             *kargoapi.Freight
		shouldErr         bool
	}{
		"idle freight": {
			clientBuilderFunc: func(b *fake.ClientBuilder) *fake.ClientBuilder {
				return b
			},
			input: &kargoapi.Freight{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fake-freight",
				},
				ID: "fake-id",
			},
			shouldErr: false,
		},
		"in-use freight": {
			clientBuilderFunc: func(b *fake.ClientBuilder) *fake.ClientBuilder {
				return b.WithObjects(
					&kargoapi.Stage{
						ObjectMeta: metav1.ObjectMeta{
							Name: "fake-stage",
						},
						Status: kargoapi.StageStatus{
							CurrentFreight: &kargoapi.SimpleFreight{
								ID: "fake-id",
							},
						},
					},
				)
			},
			input: &kargoapi.Freight{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fake-freight",
				},
				ID: "fake-id",
			},
			shouldErr: true,
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			w := newWebhook(
				tc.clientBuilderFunc(fake.NewClientBuilder().WithScheme(scheme)).
					Build(),
			)
			err := w.ValidateDelete(ctx, tc.input)
			if tc.shouldErr {
				require.Error(t, err)
				require.True(t, apierrors.IsForbidden(err))
				return
			}
			require.NoError(t, err)
		})
	}
}
