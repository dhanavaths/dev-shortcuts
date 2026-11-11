import copy
import io
import json
import sys
from contextlib import redirect_stdout
from pathlib import Path
import unittest
from unittest.mock import mock_open, patch

from prettytable import PrettyTable
import yaml


PARSER_PATH = Path(__file__).resolve().parents[1] / 'parse_application_status.py'
sys.path.insert(0, str(PARSER_PATH.parent))
import parse_application_status as parser


def make_application(kind, status):
    return {
        'metadata': {'name': 'test-app', 'namespace': 'test-ns', 'generation': 12},
        'spec': {
            'version': 'release-2',
            'workload': [{
                'manifest': {
                    'kind': kind,
                    'metadata': {'name': 'test-workload'},
                    'spec': {
                        'replicas': 7,
                        'template': {'spec': {'containers': [{
                            'name': 'test-container',
                            'image': 'registry/test-image:v2',
                            'resources': {'requests': {'cpu': '1'}},
                        }]}},
                    },
                },
            }],
        },
        'status': {
            'observedGeneration': 12,
            'applicationState': 'Available',
            'rolloutStatus': 'InProgress',
            **copy.deepcopy(status),
        },
    }


def make_v2_status():
    return {
        'aggregatedStatus': {
            'availableReplicas': 430,
            'nonSchedulableReplicas': 4,
            'readyReplicas': 430,
            'replicas': 434,
            'updatedReplicas': 417,
        },
        'targetVersionStatus': {
            'availableReplicas': 414,
            'desiredReplicas': 434,
            'nonSchedulableReplicas': 3,
            'readyReplicas': 414,
            'replicas': 417,
        },
        'clusters': [
            {
                'cluster': 'cluster-b',
                'replicas': 7,
                'observedGeneration': 11,
                'inRolloutGeneration': -1,
                'lastManifestStatusObservedTime': '2026-11-11T18:08:16Z',
                'conditions': [{
                    'type': 'Ready', 'status': 'False', 'reason': 'ManifestsNotReady',
                    'message': 'Not ready manifests:\n- test-ns/test-workload',
                }],
            },
            {
                'cluster': 'cluster-a',
                'replicas': 0,
                'observedGeneration': 12,
                'conditions': [{
                    'type': 'Ready', 'status': 'True', 'reason': 'NewVersionAvailable',
                    'message': 'All manifests are available in the cluster',
                }],
            },
        ],
        'clustersUpgrading': ['cluster-b'],
        'updatedClusters': 62,
    }


class ApplicationStatusTests(unittest.TestCase):
    def render(self, application):
        tables = []

        def capture_table():
            table = PrettyTable()
            tables.append(table)
            return table

        output = io.StringIO()
        with patch.object(parser, 'PrettyTable', side_effect=capture_table), \
                patch.object(parser.status_v2, 'PrettyTable', side_effect=capture_table):
            with redirect_stdout(output):
                parser._parse_regular_workload(
                    application, parser.get_app_type(application) == parser.daemonset
                )
        return output.getvalue(), tables

    def test_main_detects_both_versions_from_json_and_yaml(self):
        for kind in ('Deployment', 'DaemonSet'):
            for version in (1, 2):
                status = make_v2_status() if version == 2 else {'clusters': []}
                application = make_application(kind, status)
                for serializer in (json.dumps, yaml.safe_dump):
                    with self.subTest(kind=kind, version=version, serializer=serializer.__name__):
                        payload = serializer(application).encode('utf-8')
                        output = io.StringIO()
                        with patch('builtins.open', mock_open(read_data=payload)), redirect_stdout(output):
                            parser._main()
                        expected_header = 'Cluster Statuses (v2):' if version == 2 else 'Manifest Statuses:'
                        self.assertIn(expected_header, output.getvalue())
                        self.assertIn('Application Information:', output.getvalue())

    def test_detects_status_shape_not_application_release(self):
        application = make_application('Deployment', {'clusters': []})
        self.assertEqual(parser.status_v2.get_status_version(application), 1)
        for field in ('aggregatedStatus', 'targetVersionStatus'):
            with self.subTest(field=field):
                application['status'] = {field: {}}
                self.assertEqual(parser.status_v2.get_status_version(application), 2)

    def test_v2_renders_clusters_and_separate_replica_scopes(self):
        for kind in ('Deployment', 'DaemonSet', 'StatefulSet'):
            with self.subTest(kind=kind):
                application = make_application(kind, make_v2_status())
                original = copy.deepcopy(application)
                output, tables = self.render(application)
                self.assertIn('Cluster Statuses (v2):', output)
                self.assertIn('Updated clusters: 62', output)
                self.assertIn('Application Conditions:', output)
                self.assertIn('Application Information:', output)
                self.assertIn('test-image:v2', output)
                self.assertEqual(application, original)
                cluster_table, replica_table = tables[:2]
                self.assertEqual(cluster_table.rows[0], [
                    'cluster-a', 'True', 0, 12, 0, '-', '-', '-', '',
                ])
                self.assertEqual(cluster_table.rows[1], [
                    parser.YELLOW_COLOR + 'cluster-b' + parser.DEFAULT_COLOR,
                    'False', 7, 11, -1, '-', 'Yes', '2026-11-11T18:08:16Z',
                    'Not ready manifests:\n- test-ns/test-workload',
                ])
                self.assertEqual(replica_table.rows, [
                    ['Aggregated', '-', 434, 417, 430, 430, 4],
                    ['Target Version', 434, 417, '-', 414, 414, 3],
                ])

    def test_v2_rollout_generation_placeholder_preserves_other_values(self):
        for generation, expected in ((-1, '-'), (0, 0), (12, 12)):
            with self.subTest(generation=generation):
                status = make_v2_status()
                status['clusters'][0]['inRolloutGeneration'] = generation
                _, tables = self.render(make_application('Deployment', status))
                self.assertEqual(tables[0].rows[1][5], expected)
                self.assertEqual(tables[0].rows[1][4], -1)

    def test_v2_missing_counts_are_not_reported_as_zero(self):
        status = {
            'aggregatedStatus': {},
            'targetVersionStatus': {'desiredReplicas': 0, 'replicas': 0},
            'clusters': [{'cluster': 'pending-cluster'}],
        }
        _, tables = self.render(make_application('DaemonSet', status))
        self.assertEqual(tables[0].rows[0][1:], [
            'Unknown', '-', '-', '-', '-', '-', '-', '',
        ])
        self.assertEqual(tables[1].rows, [
            ['Aggregated', '-', '-', '-', '-', '-', '-'],
            ['Target Version', 0, 0, '-', '-', '-', '-'],
        ])

    def test_v2_without_clusters_still_renders_replica_summary(self):
        for field in ('aggregatedStatus', 'targetVersionStatus'):
            with self.subTest(field=field):
                output, tables = self.render(make_application('Deployment', {field: {'replicas': 5}}))
                self.assertIn('Clusters with status: 0', output)
                self.assertEqual(tables[0].rows, [])
                self.assertEqual(len(tables[1].rows), 1)
                self.assertEqual(tables[1].rows[0][2], 5)

    def test_legacy_deployment_and_statefulset_rows_are_unchanged(self):
        for kind in ('Deployment', 'StatefulSet'):
            with self.subTest(kind=kind):
                status = {'clusters': [{
                    'cluster': 'legacy-cluster',
                    'observedGeneration': 11,
                    'conditions': [{
                        'type': 'Ready', 'status': 'False', 'message': 'rollout pending',
                    }],
                    'manifestStatuses': [{
                        'kind': kind,
                        'status': {
                            'desiredReplicas': 7, 'replicas': 8, 'availableNewReplicas': 5,
                            'nonSchedulableNewReplicas': 2, 'observedGeneration': 10,
                            'collisionCount': 1,
                        },
                    }],
                }]}
                application = make_application(kind, status)
                self.assertEqual(parser.status_v2.get_status_version(application), 1)
                output, tables = self.render(application)
                self.assertIn('Manifest Statuses:', output)
                self.assertNotIn('Replica Statuses:', output)
                self.assertEqual(tables[0].rows, [
                    [parser.YELLOW_COLOR + 'legacy-cluster' + parser.DEFAULT_COLOR,
                     'False', 7, 8, 5, -2, 2, 10, -1, 1, 'rollout pending'],
                    ['TOTAL', '', 7, 8, 5, -2, '', '', '', '', ''],
                ])

    def test_legacy_daemonset_rows_are_unchanged(self):
        status = {'clusters': [{
            'cluster': 'legacy-cluster',
            'observedGeneration': 12,
            'conditions': [{'type': 'Ready', 'status': 'True'}],
            'manifestStatuses': [{
                'kind': 'DaemonSet',
                'status': {
                    'desiredNumberScheduled': 8, 'updatedNumberScheduled': 6,
                    'currentNumberScheduled': 8, 'numberReady': 5, 'numberAvailable': 4,
                    'numberMisscheduled': 1, 'observedGeneration': 10,
                },
            }],
        }]}
        output, tables = self.render(make_application('DaemonSet', status))
        self.assertIn('Manifest Statuses:', output)
        self.assertNotIn('Replica Statuses:', output)
        self.assertEqual(tables[0].rows, [
            ['legacy-cluster', 'True', 8, 6, -2, 8, 5, 4, 1, 10, 0, ''],
            ['TOTAL', '', 8, '', '', '', 5, 4, '', '', '', ''],
        ])


if __name__ == '__main__':
    unittest.main()
