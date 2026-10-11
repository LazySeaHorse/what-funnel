import uuid
import pytest
import numpy as np
from mining import Cluster, cluster_messages


def test_cluster_class():
    msg1_id = uuid.uuid4()
    msg2_id = uuid.uuid4()
    emb1 = [1.0, 0.0]
    emb2 = [0.0, 1.0]

    cluster = Cluster(msg1_id, "Hello", emb1)
    assert cluster.message_ids == [msg1_id]
    assert cluster.texts == ["Hello"]
    assert [e.tolist() for e in cluster.embeddings] == [emb1]
    assert cluster.centroid.tolist() == emb1

    cluster.add(msg2_id, "World", emb2)
    assert cluster.message_ids == [msg1_id, msg2_id]
    assert cluster.texts == ["Hello", "World"]
    assert [e.tolist() for e in cluster.embeddings] == [emb1, emb2]
    assert cluster.centroid.tolist() == pytest.approx([0.5, 0.5])


def test_cluster_centroid_is_the_running_mean():
    rng = np.random.default_rng(7)
    embs = rng.normal(size=(50, 8))
    cluster = Cluster(uuid.uuid4(), "t0", embs[0].tolist())
    for i, emb in enumerate(embs[1:], start=1):
        cluster.add(uuid.uuid4(), f"t{i}", emb.tolist())
    assert cluster.centroid.tolist() == pytest.approx(embs.mean(axis=0).tolist())


def test_cluster_messages_empty():
    assert cluster_messages([], threshold=0.85) == []


def test_cluster_messages_single():
    msg = {"id": uuid.uuid4(), "text": "Hi", "embedding": [1.0, 0.0]}
    clusters = cluster_messages([msg], threshold=0.85)
    assert len(clusters) == 1
    assert clusters[0].texts == ["Hi"]
    assert clusters[0].centroid.tolist() == [1.0, 0.0]


def test_cluster_messages_grouping():
    # 3 similar messages along x-axis (similarity ~ 0.999+)
    v_base = [1.0, 0.0]
    v_close1 = [0.99, 0.01]
    v_close2 = [0.98, -0.01]
    # 2 messages along y-axis
    v_other1 = [0.0, 1.0]
    v_other2 = [0.02, 0.99]

    msgs = [
        {"id": uuid.uuid4(), "text": "A1", "embedding": v_base},
        {"id": uuid.uuid4(), "text": "B1", "embedding": v_other1},
        {"id": uuid.uuid4(), "text": "A2", "embedding": v_close1},
        {"id": uuid.uuid4(), "text": "B2", "embedding": v_other2},
        {"id": uuid.uuid4(), "text": "A3", "embedding": v_close2},
    ]

    clusters = cluster_messages(msgs, threshold=0.85)
    assert len(clusters) == 2

    # Cluster 1 should have A1, A2, A3
    assert set(clusters[0].texts) == {"A1", "A2", "A3"}
    # Cluster 2 should have B1, B2
    assert set(clusters[1].texts) == {"B1", "B2"}


def test_cluster_messages_with_zero_vectors():
    msg1 = {"id": uuid.uuid4(), "text": "Normal", "embedding": [1.0, 0.0]}
    msg2 = {"id": uuid.uuid4(), "text": "Zero", "embedding": [0.0, 0.0]}
    msg3 = {"id": uuid.uuid4(), "text": "Normal2", "embedding": [0.99, 0.01]}

    clusters = cluster_messages([msg1, msg2, msg3], threshold=0.85)
    # Zero vector does not match msg1, forms its own cluster
    # msg3 matches msg1
    assert len(clusters) == 2
    assert set(clusters[0].texts) == {"Normal", "Normal2"}
    assert clusters[1].texts == ["Zero"]


def test_cluster_messages_high_dimension_performance():
    # Verify clustering on 1536-dimensional embeddings executes quickly
    dim = 1536
    rng = np.random.default_rng(42)

    # 10 messages: 5 around center A, 5 around center B
    center_a = rng.normal(loc=1.0, scale=0.1, size=dim)
    center_b = rng.normal(loc=-1.0, scale=0.1, size=dim)

    msgs = []
    for i in range(5):
        emb_a = (center_a + rng.normal(0, 0.01, size=dim)).tolist()
        msgs.append({"id": uuid.uuid4(), "text": f"A_{i}", "embedding": emb_a})
    for i in range(5):
        emb_b = (center_b + rng.normal(0, 0.01, size=dim)).tolist()
        msgs.append({"id": uuid.uuid4(), "text": f"B_{i}", "embedding": emb_b})

    clusters = cluster_messages(msgs, threshold=0.85)
    assert len(clusters) == 2
    c0_texts = set(clusters[0].texts)
    c1_texts = set(clusters[1].texts)
    assert (c0_texts == {f"A_{i}" for i in range(5)} and c1_texts == {f"B_{i}" for i in range(5)}) or \
           (c1_texts == {f"A_{i}" for i in range(5)} and c0_texts == {f"B_{i}" for i in range(5)})
