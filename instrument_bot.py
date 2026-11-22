"""Local-only shooting instrumentation of the frozen production Bot."""
from collections import Counter
import math
import sys

from bot import Bot, distance
import mechfight as mf


class InstrumentedBot(Bot):
    def __init__(self, hello):
        super().__init__(hello)
        self.counts = Counter()
        self.shot_locals = {}

    def shooting(self, obs, action):
        def capture(frame, event, value):
            if event == 'return' and frame.f_code is Bot.shooting.__code__:
                self.shot_locals = dict(frame.f_locals)
        sys.setprofile(capture)
        try:
            return super().shooting(obs, action)
        finally:
            sys.setprofile(None)

    def act(self, obs):
        action = super().act(obs)
        q, me = self.shot_locals, obs['self']
        actors = [a for a in obs['mechs'] if a['kind'] == 'mech']
        origin = [*me['pos'][:2], me['pos'][2] + me['stats'].get('muzzle_height', 7.)]
        visible = []
        saved = self.deadline
        self.deadline = math.inf
        for actor in actors:
            if self.visible_aim_point(actor, origin)[0] is not None:
                visible.append(actor)
        self.deadline = saved
        self.counts['total'] += 1
        self.counts['observed_mech'] += bool(actors)
        self.counts['mech_los'] += bool(visible)
        self.counts['laser_charging'] += any(w and w.get('charge', 0) > 0 for w in me['weapons'])
        target = q.get('target')
        action['profile_target'] = {'id': target[0]['id'], 'kind': target[0]['kind']} if target else {'kind': 'none'}
        gun_slot = next((i for i,w in enumerate(me['weapons']) if w and w['kind'] == 'gun'), None)
        if not target:
            self.counts['target_none'] += 1
            if not actors:
                reason = 'no_mech_observed'
            elif not visible:
                reason = 'all_mechs_occluded'
            else:
                eligible = [a for a in visible if self.ffa or self.hostile_until.get(a['id'], -1) > obs['t'] or distance(me['pos'], a['pos']) <= 46 or a.get('hp', 1400) <= 400]
                reason = 'engagement_gate' if not eligible else 'selection_range' if all(distance(me['pos'], a['pos']) >= 280 for a in eligible) else 'other_selection_gate'
        elif gun_slot is None:
            reason = 'no_gun'
        else:
            self.counts['target_' + target[0]['kind']] += 1
            gun = me['weapons'][gun_slot]
            if gun.get('reload', 0) > 0:
                reason = 'gun_reloading'
            elif action.get('reload'):
                reason = 'initiating_reload'
            elif gun.get('ammo', 0) <= 0:
                reason = 'gun_empty'
            elif action['fire'][gun_slot]:
                reason = 'trigger_cooldown' if gun.get('cooldown', 0) > .05 else 'ready_trigger'
            elif q.get('abstain'):
                reason = 'hurt_abstain'
            elif q.get('d', 0) >= self.weapon_stats(gun)['range'] - 3:
                reason = 'gun_range'
            elif not q.get('gun_aligned'):
                reason = 'aim_alignment'
            else:
                reason = 'other_fire_gate'
        self.counts['state_' + reason] += 1
        action['profile'] = dict(self.counts)
        return action


if __name__ == '__main__':
    mf.run(InstrumentedBot)
