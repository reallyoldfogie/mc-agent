#!/usr/bin/env python3
"""
Calculate all rays going in negative Z direction and determine if any hit the log block.
"""

import math

# Agent eye position
EYE_X, EYE_Y, EYE_Z = -8.0, 82.12, 7.0

# Log block position (block coordinates)
LOG_X, LOG_Y, LOG_Z = -13, 80, -11
LOG_X_MIN, LOG_X_MAX = LOG_X, LOG_X + 1
LOG_Y_MIN, LOG_Y_MAX = LOG_Y, LOG_Y + 1
LOG_Z_MIN, LOG_Z_MAX = LOG_Z, LOG_Z + 1

# Cube shell radius
R = 32

def normalize(x, y, z):
    """Normalize a 3D vector"""
    mag = math.sqrt(x*x + y*y + z*z)
    if mag == 0:
        return 0, 0, 0
    return x/mag, y/mag, z/mag

def distance(x1, y1, z1, x2, y2, z2):
    """Calculate 3D distance"""
    dx = x2 - x1
    dy = y2 - y1
    dz = z2 - z1
    return math.sqrt(dx*dx + dy*dy + dz*dz)

def ray_intersects_block(origin, direction, block_min, block_max, max_distance):
    """
    Check if a ray intersects an axis-aligned block.

    Uses parametric ray equation: P(t) = origin + t * direction
    Tests if ray passes through the block before exceeding max_distance.

    Returns: (intersects, t_entry, t_exit, entry_point, exit_point)
    """
    ox, oy, oz = origin
    dx, dy, dz = direction

    # For each axis, find the t range where the ray is within the block's bounds
    t_min = 0
    t_max = float('inf')

    # X axis
    if abs(dx) > 1e-9:
        t1 = (block_min[0] - ox) / dx
        t2 = (block_max[0] - ox) / dx
        t_xmin, t_xmax = min(t1, t2), max(t1, t2)
        t_min = max(t_min, t_xmin)
        t_max = min(t_max, t_xmax)
    else:
        # Ray is parallel to X axis, check if it's within block bounds
        if not (block_min[0] <= ox < block_max[0]):
            return False, None, None, None, None

    # Y axis
    if abs(dy) > 1e-9:
        t1 = (block_min[1] - oy) / dy
        t2 = (block_max[1] - oy) / dy
        t_ymin, t_ymax = min(t1, t2), max(t1, t2)
        t_min = max(t_min, t_ymin)
        t_max = min(t_max, t_ymax)
    else:
        if not (block_min[1] <= oy < block_max[1]):
            return False, None, None, None, None

    # Z axis
    if abs(dz) > 1e-9:
        t1 = (block_min[2] - oz) / dz
        t2 = (block_max[2] - oz) / dz
        t_zmin, t_zmax = min(t1, t2), max(t1, t2)
        t_min = max(t_min, t_zmin)
        t_max = min(t_max, t_zmax)
    else:
        if not (block_min[2] <= oz < block_max[2]):
            return False, None, None, None, None

    # Check if there's a valid intersection within radius
    if t_min < t_max and t_min >= 0 and t_min <= max_distance:
        entry_point = (ox + t_min * dx, oy + t_min * dy, oz + t_min * dz)
        exit_point = (ox + t_max * dx, oy + t_max * dy, oz + t_max * dz)
        return True, t_min, t_max, entry_point, exit_point

    return False, None, None, None, None

# Generate all rays on the back face (Z = -32)
# with focus on rays that go in negative Z direction (which they all do)
print(f"Agent eye position: ({EYE_X}, {EYE_Y}, {EYE_Z})")
print(f"Log block: ({LOG_X}, {LOG_Y}, {LOG_Z}) [{LOG_X_MIN}, {LOG_X_MAX}) × [{LOG_Y_MIN}, {LOG_Y_MAX}) × [{LOG_Z_MIN}, {LOG_Z_MAX})")
print(f"Cube radius: {R}")
print()

# Back face targets: Z = -32, X in [-32, 32], Y in [-32, 32]
back_face_targets = []
for tx in range(-R, R+1):
    for ty in range(-R, R+1):
        back_face_targets.append((tx, ty, -R))

print(f"Total rays on back face: {len(back_face_targets)}")
print()

# Check which rays would hit the log
rays_hitting_log = []

for target in back_face_targets:
    tx, ty, tz = target

    # Direction from eye to target (center of block)
    dx = tx + 0.5 - EYE_X
    dy = ty + 0.5 - EYE_Y
    dz = tz + 0.5 - EYE_Z

    # Normalize direction
    dir_x, dir_y, dir_z = normalize(dx, dy, dz)

    # Check if ray intersects log block
    block_min = (LOG_X_MIN, LOG_Y_MIN, LOG_Z_MIN)
    block_max = (LOG_X_MAX, LOG_Y_MAX, LOG_Z_MAX)

    intersects, t_entry, t_exit, entry_pt, exit_pt = ray_intersects_block(
        (EYE_X, EYE_Y, EYE_Z),
        (dir_x, dir_y, dir_z),
        block_min,
        block_max,
        R
    )

    if intersects:
        dist_to_entry = distance(EYE_X, EYE_Y, EYE_Z, entry_pt[0], entry_pt[1], entry_pt[2])
        rays_hitting_log.append({
            'target': target,
            'direction': (dir_x, dir_y, dir_z),
            't_entry': t_entry,
            'dist_to_entry': dist_to_entry,
            'entry_pt': entry_pt,
            'exit_pt': exit_pt,
        })

print(f"Rays hitting log block: {len(rays_hitting_log)}\n")

if rays_hitting_log:
    print("=" * 100)
    print("RAYS THAT HIT THE LOG BLOCK:")
    print("=" * 100)
    for ray in sorted(rays_hitting_log, key=lambda r: r['dist_to_entry']):
        tx, ty, tz = ray['target']
        dx, dy, dz = ray['direction']
        entry = ray['entry_pt']
        dist = ray['dist_to_entry']

        print(f"Target: ({tx:3d}, {ty:3d}, {tz:3d})")
        print(f"Direction: ({dx:.4f}, {dy:.4f}, {dz:.4f})")
        print(f"Entry point: ({entry[0]:.2f}, {entry[1]:.2f}, {entry[2]:.2f})")
        print(f"Distance to entry: {dist:.2f}")
        print()
else:
    print("ERROR: No rays hit the log block!")
    print()
    print("This shouldn't happen if the log is visible from the agent position.")
    print("Let's check what's closest to hitting it:")

    # Find rays that come closest to the log
    closest_rays = []
    for target in back_face_targets:
        tx, ty, tz = target
        dx = tx + 0.5 - EYE_X
        dy = ty + 0.5 - EYE_Y
        dz = tz + 0.5 - EYE_Z
        dir_x, dir_y, dir_z = normalize(dx, dy, dz)

        # Find closest approach to log center
        log_center = (LOG_X + 0.5, LOG_Y + 0.5, LOG_Z + 0.5)

        # Use parametric equation to find closest point on ray to log center
        # P(t) = eye + t * direction
        # Find t that minimizes distance to log center
        ex, ey, ez = EYE_X, EYE_Y, EYE_Z

        # Dot product of (log_center - eye) with direction
        t_closest = ((log_center[0] - ex) * dir_x +
                     (log_center[1] - ey) * dir_y +
                     (log_center[2] - ez) * dir_z)

        if t_closest >= 0 and t_closest <= R:
            closest_pt = (ex + t_closest * dir_x,
                         ey + t_closest * dir_y,
                         ez + t_closest * dir_z)
            dist_to_closest = distance(closest_pt[0], closest_pt[1], closest_pt[2],
                                      log_center[0], log_center[1], log_center[2])

            closest_rays.append({
                'target': target,
                'direction': (dir_x, dir_y, dir_z),
                'closest_pt': closest_pt,
                'min_dist': dist_to_closest,
            })

    print("Top 10 closest rays to the log block:")
    print()
    for ray in sorted(closest_rays, key=lambda r: r['min_dist'])[:10]:
        tx, ty, tz = ray['target']
        dx, dy, dz = ray['direction']
        closest = ray['closest_pt']
        dist = ray['min_dist']

        print(f"Target: ({tx:3d}, {ty:3d}, {tz:3d})")
        print(f"Direction: ({dx:.4f}, {dy:.4f}, {dz:.4f})")
        print(f"Closest point on ray: ({closest[0]:.2f}, {closest[1]:.2f}, {closest[2]:.2f})")
        print(f"Min distance to log center: {dist:.2f}")
        print()
