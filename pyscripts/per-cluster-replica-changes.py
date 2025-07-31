c_old = c_new = 0
# 07/16/2025 18:04
rm_old = {'std-cluster-sc-uks-0a6h6': 68, 'std-cluster-sc-uks-2dfjx': 62, 'std-cluster-sc-uks-4k5bx': 68, 'std-cluster-sc-uks-7na0n': 66, 'std-cluster-sc-uks-80idq': 66, 'std-cluster-sc-uks-a7x9b': 67, 'std-cluster-sc-uks-ag3x2': 67, 'std-cluster-sc-uks-b266u': 67, 'std-cluster-sc-uks-f0wrl': 65, 'std-cluster-sc-uks-fqhg0': 67, 'std-cluster-sc-uks-h9t6m': 67, 'std-cluster-sc-uks-he9zk': 67, 'std-cluster-sc-uks-kgu21': 67, 'std-cluster-sc-uks-og66v': 67, 'std-cluster-sc-uks-pawer': 66, 'std-cluster-sc-uks-pie28': 67, 'std-cluster-sc-uks-plfxj': 67, 'std-cluster-sc-uks-q5kls': 59, 'std-cluster-sc-uks-qevvf': 67, 'std-cluster-sc-uks-vgzl3': 65, 'std-cluster-sc-uks-vqy3g': 67, 'std-cluster-sc-uks-wpnu7': 45, 'std-cluster-sc-uks-zfa37': 66}
rm_new = {'std-cluster-sc-uks-0a6h6': 69, 'std-cluster-sc-uks-2dfjx': 63, 'std-cluster-sc-uks-4k5bx': 68, 'std-cluster-sc-uks-7na0n': 66, 'std-cluster-sc-uks-80idq': 68, 'std-cluster-sc-uks-a7x9b': 69, 'std-cluster-sc-uks-ag3x2': 69, 'std-cluster-sc-uks-b266u': 69, 'std-cluster-sc-uks-f0wrl': 67, 'std-cluster-sc-uks-fqhg0': 68, 'std-cluster-sc-uks-h9t6m': 68, 'std-cluster-sc-uks-he9zk': 67, 'std-cluster-sc-uks-irstm': 10, 'std-cluster-sc-uks-kgu21': 68, 'std-cluster-sc-uks-og66v': 68, 'std-cluster-sc-uks-pawer': 66, 'std-cluster-sc-uks-pie28': 69, 'std-cluster-sc-uks-plfxj': 69, 'std-cluster-sc-uks-qevvf': 68, 'std-cluster-sc-uks-vgzl3': 65, 'std-cluster-sc-uks-vqy3g': 69, 'std-cluster-sc-uks-wpnu7': 68, 'std-cluster-sc-uks-zfa37': 69}

# 07/17/2025 03:50
# oldWorkInfoToReplicas
rm_old = {"std-cluster-sc-uks-0a6h6":69, "std-cluster-sc-uks-2dfjx":63, "std-cluster-sc-uks-4k5bx":68, "std-cluster-sc-uks-7na0n":66, "std-cluster-sc-uks-80idq":68, "std-cluster-sc-uks-a7x9b":69, "std-cluster-sc-uks-ag3x2":69, "std-cluster-sc-uks-b266u":69, "std-cluster-sc-uks-f0wrl":67, "std-cluster-sc-uks-fqhg0":68, "std-cluster-sc-uks-h9t6m":68, "std-cluster-sc-uks-he9zk":67, "std-cluster-sc-uks-irstm":10, "std-cluster-sc-uks-kgu21":68, "std-cluster-sc-uks-og66v":68, "std-cluster-sc-uks-pawer":66, "std-cluster-sc-uks-pie28":69, "std-cluster-sc-uks-plfxj":69, "std-cluster-sc-uks-qevvf":68, "std-cluster-sc-uks-vgzl3":65, "std-cluster-sc-uks-vqy3g":69, "std-cluster-sc-uks-wpnu7":68, "std-cluster-sc-uks-zfa37":69}
# place result
rm_new = {"std-cluster-sc-uks-2dfjx":63, "std-cluster-sc-uks-4k5bx":68, "std-cluster-sc-uks-7na0n":66, "std-cluster-sc-uks-80idq":68, "std-cluster-sc-uks-a7x9b":69, "std-cluster-sc-uks-ag3x2":69, "std-cluster-sc-uks-b266u":69, "std-cluster-sc-uks-f0wrl":67, "std-cluster-sc-uks-fqhg0":68, "std-cluster-sc-uks-h9t6m":68, "std-cluster-sc-uks-he9zk":67, "std-cluster-sc-uks-irstm":68, "std-cluster-sc-uks-kgu21":68, "std-cluster-sc-uks-og66v":68, "std-cluster-sc-uks-pawer":66, "std-cluster-sc-uks-pie28":69, "std-cluster-sc-uks-plfxj":69, "std-cluster-sc-uks-qevvf":68, "std-cluster-sc-uks-vgzl3":65, "std-cluster-sc-uks-vqy3g":69, "std-cluster-sc-uks-wpnu7":68, "std-cluster-sc-uks-z75y7":11, "std-cluster-sc-uks-zfa37":69}

adj = 0

for k in rm_old:
    c_old += rm_old[k]

for k in rm_new:
    c_new += rm_new[k]

print("Updated or scaled up MW:")
for k in rm_new:
    if rm_new.get(k) != rm_old.get(k):
        print(k, rm_old.get(k, 0), rm_new.get(k, 0))
        adj += abs(rm_new.get(k, 0) - rm_old.get(k, 0))

print("Deleted MW:")
for k in rm_old:
    if k not in rm_new:
        print(k, rm_old.get(k, 0), rm_new.get(k, 0))

print(c_old, c_new, adj)