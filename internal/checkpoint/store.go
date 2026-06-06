/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package checkpoint persists the exact pre-sleep replica count of each workload
// out-of-band in an operator-owned Secret, so the original can be restored on wake
// and survives operator restarts.
package checkpoint

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

// Store reads and writes per-schedule checkpoint Secrets in the operator namespace.
type Store struct {
	Client    client.Client
	Namespace string
}

// Key builds the stable per-workload checkpoint key: GVK + namespace + name + UID.
// The UID guards against a recreated workload (same name, new identity) being
// restored from a stale checkpoint.
func Key(kind, namespace, name string, uid types.UID) string {
	return fmt.Sprintf("apps_v1_%s_%s_%s_%s", kind, namespace, name, uid)
}

func secretName(schedule *nyxv1alpha1.SleepSchedule) string {
	return schedule.Name + "-checkpoint"
}

// Get returns the checkpointed replica count for key, and whether it was present.
func (s *Store) Get(ctx context.Context, schedule *nyxv1alpha1.SleepSchedule, key string) (int32, bool, error) {
	sec := &corev1.Secret{}
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: secretName(schedule)}, sec)
	if apierrors.IsNotFound(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	raw, ok := sec.Data[key]
	if !ok {
		return 0, false, nil
	}
	n, err := strconv.ParseInt(string(raw), 10, 32)
	if err != nil {
		return 0, false, fmt.Errorf("corrupt checkpoint %q: %w", key, err)
	}
	return int32(n), true, nil
}

// Set records replicas for key, creating the Secret if needed. It does not
// overwrite an existing key — the first write captures the true original.
func (s *Store) Set(ctx context.Context, schedule *nyxv1alpha1.SleepSchedule, key string, replicas int32) error {
	sec := &corev1.Secret{}
	name := secretName(schedule)
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: name}, sec)
	switch {
	case apierrors.IsNotFound(err):
		sec = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: s.Namespace,
				Labels:    map[string]string{"app.kubernetes.io/managed-by": "k8s-nyx", "nyx.dev/schedule": schedule.Name},
			},
			Data: map[string][]byte{key: []byte(strconv.FormatInt(int64(replicas), 10))},
		}
		return s.Client.Create(ctx, sec)
	case err != nil:
		return err
	}
	if _, exists := sec.Data[key]; exists {
		return nil // never clobber the recorded original
	}
	if sec.Data == nil {
		sec.Data = map[string][]byte{}
	}
	sec.Data[key] = []byte(strconv.FormatInt(int64(replicas), 10))
	return s.Client.Update(ctx, sec)
}

// Delete removes the checkpoint entry for key (after a successful restore). When the
// last entry is removed the Secret is deleted to avoid leaving empty Secrets behind.
func (s *Store) Delete(ctx context.Context, schedule *nyxv1alpha1.SleepSchedule, key string) error {
	sec := &corev1.Secret{}
	name := secretName(schedule)
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: name}, sec)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, ok := sec.Data[key]; !ok {
		return nil
	}
	delete(sec.Data, key)
	if len(sec.Data) == 0 {
		return client.IgnoreNotFound(s.Client.Delete(ctx, sec))
	}
	return s.Client.Update(ctx, sec)
}
