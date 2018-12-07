local dpos = {}

dpos.A = {}
dpos.A.network = dpos_network.new()
dpos.A.arbitrators = arbitrators.new()
dpos.A.manager = dpos_manager.new(dpos.A.network, dpos.A.arbitrators, 0)

dpos.B = {}
dpos.B.network = dpos_network.new()
dpos.B.arbitrators = arbitrators.new()
dpos.B.manager = dpos_manager.new(dpos.B.network, dpos.B.arbitrators, 1)

dpos.C = {}
dpos.C.network = dpos_network.new()
dpos.C.arbitrators = arbitrators.new()
dpos.C.manager = dpos_manager.new(dpos.C.network, dpos.C.arbitrators, 2)

dpos.D = {}
dpos.D.network = dpos_network.new()
dpos.D.arbitrators = arbitrators.new()
dpos.D.manager = dpos_manager.new(dpos.D.network, dpos.D.arbitrators, 3)

dpos.E = {}
dpos.E.network = dpos_network.new()
dpos.E.arbitrators = arbitrators.new()
dpos.E.manager = dpos_manager.new(dpos.E.network, dpos.E.arbitrators, 4)

dpos.current_arbitrators = { dpos.A, dpos.B, dpos.C, dpos.D, dpos.E }

function dpos.set_on_duty(index)
    for i = 1, 5 do
        if index == i
        then
            dpos.current_arbitrators[i].manager:set_on_duty(true)
        else
            dpos.current_arbitrators[i].manager:set_on_duty(false)
        end
    end
end

return dpos