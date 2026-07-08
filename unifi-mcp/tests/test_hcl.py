from unifi_mcp.hcl import render_resource, resource_filename


def test_render_simple_network():
    out = render_resource(
        "unifi_network",
        "iot",
        {
            "name": "iot",
            "purpose": "corporate",
            "subnet": "10.0.30.1/24",
            "vlan_id": 30,
            "dhcp_enabled": True,
        },
    )
    assert 'resource "unifi_network" "iot"' in out
    assert 'name = "iot"' in out
    assert "vlan_id = 30" in out
    assert "dhcp_enabled = true" in out


def test_render_lists_and_interpolation():
    out = render_resource(
        "unifi_firewall_rule",
        "block_iot",
        {
            "name": "block_iot",
            "action": "drop",
            "ruleset": "LAN_IN",
            "rule_index": 2010,
            "src_firewall_group_ids": ["${unifi_firewall_group.iot.id}"],
        },
    )
    assert "src_firewall_group_ids = [" in out
    assert "${unifi_firewall_group.iot.id}" in out


def test_filename_round_trip():
    assert resource_filename("unifi_network", "iot") == "unifi_network.iot.tf"


def test_rejects_bad_identifier():
    import pytest

    with pytest.raises(ValueError):
        render_resource("unifi_network", "bad name", {"name": "x"})
    with pytest.raises(ValueError):
        render_resource("unifi_network", "ok", {"bad key": "x"})
