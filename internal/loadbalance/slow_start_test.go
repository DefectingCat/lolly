package loadbalance

import "testing"

func TestTarget_GetEffectiveWeight(t *testing.T) {
	tests := []struct {
		name            string
		weight          int
		effectiveWeight int64
		want            int
	}{
		{
			name:            "no slow start",
			weight:          10,
			effectiveWeight: 0,
			want:            10,
		},
		{
			name:            "slow start in progress",
			weight:          10,
			effectiveWeight: 5,
			want:            5,
		},
		{
			name:            "slow start at 1",
			weight:          10,
			effectiveWeight: 1,
			want:            1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &Target{
				Weight: tt.weight,
			}
			target.EffectiveWeight.Store(tt.effectiveWeight)

			got := target.GetEffectiveWeight()
			if got != tt.want {
				t.Errorf("GetEffectiveWeight() = %d, want %d", got, tt.want)
			}
		})
	}
}
