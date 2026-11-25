
        local _G = _G
        local error = error
        local coroutine = coroutine
        local setmetatable = setmetatable

        -- maps a coroutine to its sandboxed globals
        --
        -- keys are weak because a coroutine can be dropped without ever running the __close below
        -- i.e. one abandoned while suspended and never closed, or a server runner destroyed together with its actor
        -- ServerLuaCoroutineRunner runs lua_closethread() on runners that raise or get closed, but runs no lua at actor teardown
        -- its entry would otherwise stay forever, and because the key is the coroutine itself a strong reference would also keep the dead coroutine alive and unreclaimable
        --
        -- with weak keys the collector drops both once nothing else refers to the coroutine
        -- __close stays as the prompt path that frees the entry as soon as the coroutine finishes, raises, or gets closed

        local _RSVD_NAME_G_threadSandBox = setmetatable({}, {__mode = 'k'})

        function getTLSTable()
            local threadId, inMainThread = coroutine.running()
            if inMainThread then
                error('call getTLSTable() in main thread')
            else
                if _RSVD_NAME_G_threadSandBox[threadId] == nil then
                    _RSVD_NAME_G_threadSandBox[threadId] = {}
                end
                return _RSVD_NAME_G_threadSandBox[threadId]
            end
        end

        local _RSVD_NAME_clearTLSTable = {}
        setmetatable(_RSVD_NAME_clearTLSTable, {__close = function()
            local threadId, inMainThread = coroutine.running()
            if inMainThread then
                error('call clearTLSTable() in main thread')
            else
                _RSVD_NAME_G_threadSandBox[threadId] = nil
            end
        end})

        function autoClearTLSTable()
            return _RSVD_NAME_clearTLSTable
        end

        _RSVD_NAME_replaceEnvMetaTable = {
            __index = function(_, key)
                local threadId, inMainThread = coroutine.running()
                if not inMainThread then
                    if _RSVD_NAME_G_threadSandBox[threadId] ~= nil and _RSVD_NAME_G_threadSandBox[threadId][key] ~= nil then
                        return _RSVD_NAME_G_threadSandBox[threadId][key]
                    end
                end
                return _G[key]
            end,

            __newindex = function(_, key, value)
                local threadId, inMainThread = coroutine.running()
                if inMainThread then
                    _G[key] = value
                else
                    if _RSVD_NAME_G_threadSandBox[threadId] == nil then
                        _RSVD_NAME_G_threadSandBox[threadId] = {}
                    end
                    _RSVD_NAME_G_threadSandBox[threadId][key] = value
                end
            end
        }
    